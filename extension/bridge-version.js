// Restart only when the installed Native Host matches the extension but its
// currently running Bridge is still from the previous installation.
export function needsBridgeRestart(extensionVersion, nativeHostVersion, bridgeVersion) {
  return Boolean(extensionVersion && bridgeVersion &&
    nativeHostVersion === extensionVersion && bridgeVersion !== extensionVersion);
}
