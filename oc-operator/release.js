#!/usr/bin/env node

const YAML = require("yaml")
const fs = require("fs")
const glob = require("glob")
const { exit } = require("process")

// read manifest file passed as argument
const version = "0.9.0-rc.8"
const file = fs.readFileSync("../config/release/tf-controller.all.yaml", "utf8")
const documents = YAML.parseAllDocuments(file)

// containerImage for CSV
const CONTROLLER_IMAGE = "ghcr.io/weaveworks/tf-controller:v" + version

const kindMap = {
  Role: "role",
  RoleBinding: "rolebinding",
  ClusterRoleBinding: "clusterrolebinding",
  Deployment: "deployment",
  CustomResourceDefinition: "crd",
  Service: "service",
  ClusterRole: "clusterrole",
  ServiceAccount: "serviceaccount",
}

// setup directory for new version
const packagePath = "./tf-controller"
const newVersionDir = `${packagePath}/${version}/`

if (!fs.existsSync(newVersionDir)) {
  fs.mkdirSync(newVersionDir)
}
const manifestsDir = `${newVersionDir}/manifests`
if (!fs.existsSync(manifestsDir)) {
  fs.mkdirSync(manifestsDir)
}
const metadataDir = `${newVersionDir}/metadata`
if (!fs.existsSync(metadataDir)) {
  fs.mkdirSync(metadataDir)
}

// update annotations
const annotations = YAML.parse(
  fs.readFileSync("./templates/annotations.yaml", "utf-8")
)
fs.writeFileSync(`${metadataDir}/annotations.yaml`, YAML.stringify(annotations))
const csv = YAML.parse(
  fs.readFileSync("./templates/clusterserviceversion.yaml", "utf-8")
)

const deployments = []
const crds = []
documents
  .filter((d) => d.contents)
  .map((d) => YAML.parse(String(d)))
  .filter((o) => o.kind !== "NetworkPolicy" && o.kind !== "Namespace") // not supported by operator-sdk
  .map((o) => {
    delete o.metadata.namespace
    switch (o.kind) {
      case "Role":
      case "RoleBinding":
      case "ClusterRoleBinding":
      case "ClusterRole":
      case "SecurityContextConstraints":
      case "Service":
        const filename = `${o.metadata.name}.${kindMap[o.kind]}.yaml`
        fs.writeFileSync(`${manifestsDir}/${filename}`, YAML.stringify(o))
        break
      case "Deployment":
        let deployment = {
          name: o.metadata.name,
          label: o.metadata.labels,
          spec: o.spec,
        }
        if (o.spec.template.spec.containers[0].env[1].name === "RUNNER_POD_IMAGE") {
          o.spec.template.spec.containers[0].env[1].value = "ghcr.io/weaveworks/tf-runner:v" + version
        }
        deployments.push(deployment)
        break
      case "CustomResourceDefinition":
        crds.push(o)
        const crdFileName = `${o.spec.names.singular}.${kindMap[o.kind]}.yaml`
        fs.writeFileSync(`${manifestsDir}/${crdFileName}`, YAML.stringify(o))
        break
      case "ServiceAccount":
        // CK: removed ServiceAccount because it's recently broke the Kiwi test
        // if(o.metadata.name === "tf-runner") {
        //  const filename = `${o.metadata.name}.${kindMap[o.kind]}.yaml`
        //  fs.writeFileSync(`${manifestsDir}/${filename}`, YAML.stringify(o))
        // }
        break
      default:
        console.warn(
          "UNSUPPORTED KIND - you must explicitly ignore it or handle it",
          o.kind,
          o.metadata.name
        )
        process.exit(1)
        break
    }
  })

// Update ClusterServiceVersion
csv.spec.install.spec.deployments = deployments
csv.metadata.name = `tf-controller.v${version}`
csv.metadata.annotations.containerImage = CONTROLLER_IMAGE
csv.spec.version = version
csv.spec.minKubeVersion = "1.19.0"
csv.spec.maturity = "stable"
csv.spec.customresourcedefinitions.owned = []

crds.forEach((crd) => {
  crd.spec.versions.forEach((v) => {
    csv.spec.customresourcedefinitions.owned.push({
      name: crd.metadata.name,
      displayName: crd.spec.names.kind,
      kind: crd.spec.names.kind,
      version: v.name,
      description: crd.spec.names.kind,
    })
  })
})

const csvFileName = `tf-controller.v${version}.clusterserviceversion.yaml`
fs.writeFileSync(`${manifestsDir}/${csvFileName}`, YAML.stringify(csv))
