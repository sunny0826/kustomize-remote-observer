#!/bin/sh

APP="Kustomize.app"
mkdir -p $APP/Contents/{MacOS,Resources}
go build -o $APP/Contents/MacOS/kustomize
cat > $APP/Contents/Info.plist << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleExecutable</key>
	<string>kustomize</string>
	<key>CFBundleIconFile</key>
	<string>icon.icns</string>
	<key>CFBundleIdentifier</key>
	<string>io.guoxudong.kustomize-remote-observer</string>
	<key>NSHighResolutionCapable</key>
  <true/>
	<key>LSUIElement</key>
  <string>1</string>
</dict>
</plist>
EOF
cp icons/icon.icns $APP/Contents/Resources/icon.icns
cp -r assets $APP/Contents/Resources/assets
cp -r views $APP/Contents/Resources/views
find $APP
