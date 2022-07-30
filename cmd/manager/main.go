/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"os"
	"time"

	"github.com/weaveworks/tf-controller/mtls"
	"github.com/weaveworks/tf-controller/runner"

	"github.com/fluxcd/pkg/runtime/client"
	"github.com/fluxcd/pkg/runtime/events"
	"github.com/fluxcd/pkg/runtime/leaderelection"
	"github.com/fluxcd/pkg/runtime/logger"
	"github.com/fluxcd/pkg/runtime/metrics"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	flag "github.com/spf13/pflag"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	"github.com/weaveworks/tf-controller/controllers"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	ctrl "sigs.k8s.io/controller-runtime"
	crtlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	//+kubebuilder:scaffold:imports
)

const controllerName = "tf-controller"

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(sourcev1.AddToScheme(scheme))
	utilruntime.Must(infrav1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var (
		metricsAddr            string
		eventsAddr             string
		healthAddr             string
		concurrent             int
		requeueDependency      time.Duration
		clientOptions          client.Options
		logOptions             logger.Options
		leaderElectionOptions  leaderelection.Options
		watchAllNamespaces     bool
		httpRetry              int
		caValidityDuration     time.Duration
		certValidityDuration   time.Duration
		rotationCheckFrequency time.Duration
		runnerGRPCPort         int
	)

	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&eventsAddr, "events-addr", "", "The address of the events receiver.")
	flag.StringVar(&healthAddr, "health-addr", ":9440", "The address the health endpoint binds to.")
	flag.IntVar(&concurrent, "concurrent", 4, "The number of concurrent terraform reconciles.")
	flag.DurationVar(&requeueDependency, "requeue-dependency", 30*time.Second, "The interval at which failing dependencies are reevaluated.")
	flag.BoolVar(&watchAllNamespaces, "watch-all-namespaces", true,
		"Watch for custom resources in all namespaces, if set to false it will only watch the runtime namespace.")
	flag.IntVar(&httpRetry, "http-retry", 9, "The maximum number of retries when failing to fetch artifacts over HTTP.")
	flag.DurationVar(&caValidityDuration, "ca-cert-validity-duration", time.Hour*24*7,
		"The duration that the ca certificate certificates should be valid for. Default is 1 week.")
	flag.DurationVar(&certValidityDuration, "cert-validity-duration", 6*time.Hour,
		"The duration that the mTLS certificate that the runner pod should be valid for.")
	flag.DurationVar(&rotationCheckFrequency, "cert-rotation-check-frequency", 30*time.Minute,
		"The interval that the mTLS certificate rotator should check the certificate validity.")
	flag.IntVar(&runnerGRPCPort, "runner-grpc-port", 30000, "The port which will be exposed on the runner pod for gRPC connections.")

	clientOptions.BindFlags(flag.CommandLine)
	logOptions.BindFlags(flag.CommandLine)
	leaderElectionOptions.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(logger.NewLogger(logOptions))
	// ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	metricsRecorder := metrics.NewRecorder()
	crtlmetrics.Registry.MustRegister(metricsRecorder.Collectors()...)

	runtimeNamespace := os.Getenv("RUNTIME_NAMESPACE")

	watchNamespace := ""
	if !watchAllNamespaces {
		watchNamespace = runtimeNamespace
	}

	restConfig := client.GetConfigOrDie(clientOptions)
	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                        scheme,
		MetricsBindAddress:            metricsAddr,
		HealthProbeBindAddress:        healthAddr,
		Port:                          9443,
		LeaderElection:                leaderElectionOptions.Enable,
		LeaderElectionReleaseOnCancel: leaderElectionOptions.ReleaseOnCancel,
		LeaseDuration:                 &leaderElectionOptions.LeaseDuration,
		RenewDeadline:                 &leaderElectionOptions.RenewDeadline,
		RetryPeriod:                   &leaderElectionOptions.RetryPeriod,
		LeaderElectionID:              "1953de50.contrib.fluxcd.io",
		Namespace:                     watchNamespace,
		Logger:                        ctrl.Log,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	var eventRecorder *events.Recorder
	if eventRecorder, err = events.NewRecorder(mgr, ctrl.Log, eventsAddr, controllerName); err != nil {
		setupLog.Error(err, "unable to create event recorder")
		os.Exit(1)
	}

	signalHandlerContext := ctrl.SetupSignalHandler()

	certsReady := make(chan struct{})
	rotator := &mtls.CertRotator{
		Ready:                         certsReady,
		CAName:                        "tf-controller",
		CAOrganization:                "weaveworks",
		DNSName:                       "tf-controller",
		CAValidityDuration:            caValidityDuration,
		RotationCheckFrequency:        rotationCheckFrequency,
		LookaheadInterval:             2 * time.Hour,
		TriggerCARotation:             make(chan mtls.Trigger),
		TriggerNamespaceTLSGeneration: make(chan mtls.Trigger),
	}

	const localHost = "localhost"
	if os.Getenv("INSECURE_LOCAL_RUNNER") == "1" {
		rotator.CAName = localHost
		rotator.CAOrganization = localHost
		rotator.DNSName = localHost
	}

	if err := mtls.AddRotator(signalHandlerContext, mgr, rotator); err != nil {
		setupLog.Error(err, "unable to set up cert rotation")
		os.Exit(1)
	}

	reconciler := &controllers.TerraformReconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		EventRecorder:   eventRecorder,
		MetricsRecorder: metricsRecorder,
		StatusPoller:    polling.NewStatusPoller(mgr.GetClient(), mgr.GetRESTMapper(), polling.Options{}),
		CertRotator:     rotator,
		RunnerGRPCPort:  runnerGRPCPort,
	}

	if err = reconciler.SetupWithManager(mgr, concurrent, httpRetry); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Terraform")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if os.Getenv("INSECURE_LOCAL_RUNNER") == "1" {
		runnerServer := &runner.TerraformRunnerServer{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}
		go func() {
			err := mtls.StartGRPCServerForTesting(runnerServer, "flux-system", "localhost:30000", mgr, rotator)
			if err != nil {
				setupLog.Error(err, "unable to start runner server")
				os.Exit(1)
			}
		}()
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(signalHandlerContext); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
