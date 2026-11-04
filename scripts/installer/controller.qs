// Adapted from VidOtter's single-product installer controller; see third_party/vidotter.
function Controller() {
    if (installer.isInstaller() && !installer.isCommandLineInstance()) {
        installer.setDefaultPageVisible(QInstaller.ComponentSelection, false);
        installer.setDefaultPageVisible(QInstaller.StartMenuSelection, false);
    }
}
Controller.prototype.IntroductionPageCallback = function() {
    if (!installer.isInstaller()) return;
    var page = gui.currentPageWidget();
    if (page && page.MessageLabel) {
        page.MessageLabel.setText("Install SyndiRelay to distribute articles from your local content. Your data and existing configuration remain on this computer.\nYour local data and configuration are retained when updating or uninstalling.");
    }
};
