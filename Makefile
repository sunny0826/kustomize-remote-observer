.PHONY: build clean

app_build:
	./build-macos.sh

build: app_build
	go run build/build_dmg.go -name "Kustomize" -dmg "dmg/template.dmg"

clean:
	rm -rf Kustomize.app
	rm -rf Kustomize.dmg