#!/bin/bash
set -e

APP="Ruuvi Listener.app"

echo "Packaging..."
fyne package -os darwin -name "Ruuvi Listener" -app-id com.hunttis.ruuvilistener --use-raw-icon

echo "Patching Info.plist with Bluetooth permissions..."
/usr/libexec/PlistBuddy -c "Add :NSBluetoothAlwaysUsageDescription string 'Ruuvi Listener needs Bluetooth to scan for Ruuvi sensor tags.'" "$APP/Contents/Info.plist" 2>/dev/null || \
/usr/libexec/PlistBuddy -c "Set :NSBluetoothAlwaysUsageDescription 'Ruuvi Listener needs Bluetooth to scan for Ruuvi sensor tags.'" "$APP/Contents/Info.plist"

/usr/libexec/PlistBuddy -c "Add :NSBluetoothPeripheralUsageDescription string 'Ruuvi Listener needs Bluetooth to scan for Ruuvi sensor tags.'" "$APP/Contents/Info.plist" 2>/dev/null || \
/usr/libexec/PlistBuddy -c "Set :NSBluetoothPeripheralUsageDescription 'Ruuvi Listener needs Bluetooth to scan for Ruuvi sensor tags.'" "$APP/Contents/Info.plist"

echo "Copying config files..."
cp config.json tags.json "$APP/Contents/Resources/"

echo "Done. Launch with: open \"$APP\""
