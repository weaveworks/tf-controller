release:
	( cd .. && make release-manifests )
	./release.js
	operator-sdk bundle validate --select-optional name=operatorhub --verbose "tf-controller/$$(yq e '.images[0].newTag | sub("v","")' ../config/manager/kustomization.yaml)"

opm-index: release
	./opm-index.sh
	yq e -i ".spec.startingCSV=\"tf-controller.v$$(yq e '.images[0].newTag | sub("v","")' ../config/manager/kustomization.yaml)\"" \
	test/004-operator-subscription.yaml

test: opm-index
	bash -x ./test.sh
