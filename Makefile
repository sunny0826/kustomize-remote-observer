.PHONY: build clean

APP_NAME="Kustomize"
DMG_TEMPLATE="dmg/template.dmg"

app_build:
	./build-macos.sh

build: app_build
	go run build/build_dmg.go -name $(APP_NAME) -dmg $(DMG_TEMPLATE)

clean:
	rm -rf Kustomize.app
	rm -rf Kustomize.dmg