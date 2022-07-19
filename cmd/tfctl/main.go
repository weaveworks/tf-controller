package main

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/weaveworks/tf-controller/tfctl"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var (
	// BuildSHA is the tfctl version
	BuildSHA string

	// BuildVersion is the tfctl build version
	BuildVersion string
)

var defaultNamespace = "flux-system"
var kubeconfigArgs = genericclioptions.NewConfigFlags(false)

func main() {
	cmd := newRootCommand()
	cobra.CheckErr(cmd.Execute())
}

func newRootCommand() *cobra.Command {
	app := tfctl.New(BuildSHA, BuildVersion)

	config := viper.New()

	rootCmd := &cobra.Command{
		Use:           "tfctl",
		SilenceErrors: false,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			k8sConfig, err := kubeconfigArgs.ToRESTConfig()
			if err != nil {
				return err
			}
			return app.Init(k8sConfig, config)
		},
	}

	configureDefaultNamespace()

	// flags
	rootCmd.PersistentFlags().String("terraform", "/usr/bin/terraform", "The location of the terraform binary.")
	kubeconfigArgs.AddFlags(rootCmd.PersistentFlags())

	// bind flags to config
	config.BindPFlags(rootCmd.PersistentFlags())

	rootCmd.AddCommand(buildVersionCmd(app))
	rootCmd.AddCommand(buildPlanGroup(app))
	rootCmd.AddCommand(buildInstallCmd(app))
	rootCmd.AddCommand(buildUninstallCmd(app))
	rootCmd.AddCommand(buildReconcileCmd(app))
	rootCmd.AddCommand(buildSuspendCmd(app))
	rootCmd.AddCommand(buildResumeCmd(app))
	rootCmd.AddCommand(buildGetGroup(app))
	rootCmd.AddCommand(buildDeleteCmd(app))
	rootCmd.AddCommand(buildCreateCmd(app))
	rootCmd.AddCommand(buildForceUnlockCmd(app))

	return rootCmd
}

func buildVersionCmd(app *tfctl.CLI) *cobra.Command {
	install := &cobra.Command{
		Use:   "version",
		Short: "Prints tf-controller and tfctl version information",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Version(os.Stdout)
		},
	}
	install.Flags().String("version", "", "The version of tf-controller to install.")
	viper.BindPFlag("version", install.Flags().Lookup("version"))
	return install
}

var installExamples = `
  # Install the Terraform controller
  tfctl install --namespace=flux-system

  # Generate the Terraform controller manifests and print them to stdout
  tfctl install --namespace=flux-system --export

  # Install a specific version of the Terraform controller
  tfctl install --namespace=flux-system --version=v0.9.3
`

func buildInstallCmd(app *tfctl.CLI) *cobra.Command {
	install := &cobra.Command{
		Use:     "install",
		Short:   "Install the tf-controller",
		Long:    "Install the tf-controller resources in the specified namespace",
		Example: strings.Trim(installExamples, "\n"),
		Args:    cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Install(os.Stdout, viper.GetString("version"), viper.GetBool("export"))
		},
	}
	install.Flags().String("version", "", "The version of tf-controller to install.")
	install.Flags().Bool("export", false, "Print installation manifests to stdout")
	viper.BindPFlags(install.Flags())
	return install
}

func buildUninstallCmd(app *tfctl.CLI) *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall the tf-controller",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Uninstall(os.Stdout)
		},
	}
}

var reconcileExamples = `
  # Reconcile a Terraform resource
  tfctl reconcile --namespace=default my-resource
`

func buildReconcileCmd(app *tfctl.CLI) *cobra.Command {
	return &cobra.Command{
		Use:     "reconcile NAME",
		Short:   "Trigger a reconcile of the provided resource",
		Example: strings.Trim(reconcileExamples, "\n"),
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Reconcile(os.Stdout, args[0])
		},
	}
}

var suspendExamples = `
  # Suspend reconciliation for a Terraform resource
  tfctl suspend my-resource
`

func buildSuspendCmd(app *tfctl.CLI) *cobra.Command {
	return &cobra.Command{
		Use:     "suspend NAME",
		Short:   "Suspend reconciliation for the provided resource",
		Example: strings.Trim(suspendExamples, "\n"),
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Suspend(os.Stdout, args[0])
		},
	}
}

var resumeExamples = `
  # Resume reconciliation for a Terraform resource
  tfctl resume my-resource
`

func buildResumeCmd(app *tfctl.CLI) *cobra.Command {
	return &cobra.Command{
		Use:     "resume NAME",
		Short:   "Resume reconciliation for the provided resource",
		Args:    cobra.ExactArgs(1),
		Example: strings.Trim(resumeExamples, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Resume(os.Stdout, args[0])
		},
	}
}

func buildPlanGroup(app *tfctl.CLI) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Plan a Terraform configuration",
	}
	cmd.AddCommand(buildPlanShowCmd(app))
	cmd.AddCommand(buildPlanApproveCmd(app))
	return cmd
}

var planShowExamples = `
  # Show the plan for a Terraform resource
  tfctl plan show my-resource
`

func buildPlanShowCmd(app *tfctl.CLI) *cobra.Command {
	return &cobra.Command{
		Use:     "show NAME",
		Short:   "Show pending Terraform plan",
		Example: strings.Trim(planShowExamples, "\n"),
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.ShowPlan(os.Stdout, args[0])
		},
	}
}

var planApproveExamples = `
  # Approve the plan for a Terraform resource
  tfctl plan approve my-resource
`

func buildPlanApproveCmd(app *tfctl.CLI) *cobra.Command {
	return &cobra.Command{
		Use:     "approve NAME",
		Short:   "Approve pending Terraform plan",
		Example: strings.Trim(planApproveExamples, "\n"),
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.ApprovePlan(os.Stdout, args[0])
		},
	}
}

var getExamples = `
  # List all Terraform resources in the given namespace
  tfctl get --namespace=default
`

func buildGetGroup(app *tfctl.CLI) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get",
		Short:   "Get Terraform resources",
		Example: strings.Trim(getExamples, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Get(os.Stdout)
		},
	}
	cmd.AddCommand(buildGetTerraformCmd(app))
	return cmd
}

var getTerraformExamples = `
  # Show a specific Terraform resource
  tfctl get my-resource
`

func buildGetTerraformCmd(app *tfctl.CLI) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get NAME",
		Short:   "Get a Terraform resource",
		Example: strings.Trim(getTerraformExamples, "\n"),
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.GetTerraform(os.Stdout, args[0])
		},
	}
	return cmd
}

var deleteExamples = `
  # Delete a Terraform resource
  tfctl delete my-resource
`

func buildDeleteCmd(app *tfctl.CLI) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete NAME",
		Short:   "Delete a Terraform resource",
		Example: strings.Trim(getTerraformExamples, "\n"),
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.DeleteTerraform(os.Stdout, args[0])
		},
	}
	return cmd
}

var createExamples = `
  # Create a Terraform resource in the default namespace
  tfctl create -n default my-resource --source GitRepository/my-project --path ./terraform --interval 15m

  # Generate a Terraform resource manifest
  tfctl create -n default my-resource --source GitRepository/my-project --path ./terraform --interval 15m --export
`

func buildCreateCmd(app *tfctl.CLI) *cobra.Command {
	create := &cobra.Command{
		Use:     "create NAME",
		Short:   "Create a Terraform resource",
		Example: strings.Trim(createExamples, "\n"),
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Create(os.Stdout,
				args[0],
				viper.GetString("namespace"),
				viper.GetString("path"),
				viper.GetString("source"),
				viper.GetString("interval"),
				viper.GetBool("export"))
		},
	}
	create.Flags().String("path", "", "")
	create.Flags().String("source", "", "")
	create.Flags().String("interval", "", "")
	create.Flags().Bool("export", false, "Print generated Terraform resource to stdout")
	viper.BindPFlags(create.Flags())
	return create
}

func configureDefaultNamespace() {
	*kubeconfigArgs.Namespace = defaultNamespace
	fromEnv := os.Getenv("FLUX_SYSTEM_NAMESPACE")
	if fromEnv != "" {
		kubeconfigArgs.Namespace = &fromEnv
	}
}

var forceUnlockExample = `
	# Unlock Terraform resource "aws-security-group" with lock id "f2ab685b-f84d-ac0b-a125-378a22877e8d" in the default namespace
	tfctl force-unlock aws-security-group -n default --lock-id="f2ab685b-f84d-ac0b-a125-378a22877e8d"
`

func buildForceUnlockCmd(app *tfctl.CLI) *cobra.Command {
	forceUnlock := &cobra.Command{
		Use:     "force-unlock",
		Short:   "Force unlock a locked Terraform State",
		Example: strings.Trim(forceUnlockExample, "\n"),
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.ForceUnlock(
				os.Stdout,
				args[0],
				viper.GetString("lock-id"),
			)
		},
	}
	forceUnlock.Flags().String("lock-id", "", "Set the lock-id that currently holds the lock of the terraform state e.g. f2ab685b-f84d-ac0b-a125-378a22877e8d")
	viper.BindPFlags(forceUnlock.Flags())
	return forceUnlock
}
