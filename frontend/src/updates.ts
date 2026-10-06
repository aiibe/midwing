import * as app from '../wailsjs/go/main/App';
import { BrowserOpenURL } from '../wailsjs/runtime/runtime';

export function setupUpdates() {
    if ('go' in window) {
        void app.Version().then(version => {
            const label = document.querySelector<HTMLElement>('#app-version')!;
            label.textContent = `v${version}`;
            label.hidden = !version;
        }).catch(() => { /* Browser previews have no app version. */ });
    }
    const notice = document.querySelector<HTMLButtonElement>('#update-notice')!;
    let checking = false;
    let downloadURL = '';
    async function check() {
        if (checking || !('go' in window)) return;
        checking = true;
        try {
            const update = await app.CheckForUpdate();
            downloadURL = update.downloadURL;
            notice.hidden = !downloadURL;
            notice.textContent = downloadURL ? `Version ${update.version} available ↗` : '';
        } catch { /* Offline checks leave the existing notice intact. */ }
        finally { checking = false; }
    }
    notice.addEventListener('click', () => { if (downloadURL) BrowserOpenURL(downloadURL); });
    const timer = window.setInterval(() => { void check(); }, 60 * 60 * 1000);
    window.addEventListener('focus', check);
    window.addEventListener('beforeunload', () => window.clearInterval(timer));
    void check();
}
