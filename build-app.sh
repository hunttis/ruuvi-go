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

echo "Copying config.json to bundle..."
# config.json is read from ~/Library/Application Support/RuuviListener/ first;
# the bundle copy is a fallback for first-time setup.
# tags.json is NOT copied — it lives permanently in Application Support.
if [ -f config.json ]; then
    cp config.json "$APP/Contents/Resources/"
else
    echo "  Warning: config.json not found in current directory, skipping."
fi

echo "Done. Launch with: open \"$APP\""
echo "Note: tag names are stored in ~/Library/Application Support/RuuviListener/tags.json"
