import * as app from '../wailsjs/go/main/App';
import type { clisetup } from '../wailsjs/go/models';
import { ClipboardSetText } from '../wailsjs/runtime/runtime';

export function setupCLI() {
    const section = document.querySelector<HTMLElement>('#cli-setup')!;
    const status = section.querySelector<HTMLElement>('#cli-status')!;
    const message = section.querySelector<HTMLElement>('#cli-message')!;
    const error = section.querySelector<HTMLElement>('#cli-error')!;
    const enabled = section.querySelector<HTMLInputElement>('#cli-enabled')!;
    const retry = section.querySelector<HTMLButtonElement>('#cli-retry')!;
    const notice = document.querySelector<HTMLButtonElement>('#cli-notice')!;
    let busy = false;
    let refreshPending = false;
    let current: clisetup.Status | undefined;

    function render(next: clisetup.Status) {
        current = next;
        const attention = !!next.setupError || (next.autoSetupDisabled ? next.installed : !next.installed);
        status.textContent = !next.available ? 'Unavailable' : attention ? 'Needs attention'
            : next.autoSetupDisabled ? 'Disabled' : 'Ready';
        message.textContent = next.setupError ? 'CLI setup failed. Open Details or retry setup.' : next.message;
        section.querySelector<HTMLElement>('#cli-controls')!.hidden = !next.available;
        enabled.checked = !next.autoSetupDisabled;
        section.querySelector<HTMLElement>('#cli-destination')!.textContent = next.destination;
        section.querySelector<HTMLElement>('#cli-target')!.textContent = next.target;
        section.querySelector<HTMLElement>('#cli-path')!.textContent = next.pathCommand;
        section.querySelector<HTMLElement>('#cli-existing-row')!.hidden = !next.existingTarget;
        section.querySelector<HTMLElement>('#cli-existing')!.textContent = next.existingTarget;
        error.textContent = next.setupError;
        retry.hidden = !next.available || !attention;
        notice.hidden = !next.available || !attention;
        notice.textContent = 'CLI needs attention';
    }

    async function read(action?: string) {
        if (busy) {
            if (!action) refreshPending = true;
            return;
        }
        if (!('go' in window)) {
            status.textContent = 'Unavailable';
            return;
        }
        busy = true;
        enabled.disabled = retry.disabled = true;
        try {
            render(action ? await app.ChangeCLISetup(action) : await app.CLISetupStatus());
        } catch (e) {
            // A failed link change may still have saved the user's preference.
            if (action) {
                try { render(await app.CLISetupStatus()); } catch { /* Retain the last known status. */ }
            }
            enabled.checked = current ? !current.autoSetupDisabled : false;
            status.textContent = 'Needs attention';
            message.textContent = 'Could not complete CLI setup. Open Details or retry setup.';
            error.textContent = String(e);
            retry.hidden = false;
            notice.hidden = false;
            notice.textContent = 'CLI needs attention';
        } finally {
            busy = false;
            enabled.disabled = !current?.available;
            retry.disabled = false;
            if (refreshPending) {
                refreshPending = false;
                void read();
            }
        }
    }

    enabled.addEventListener('change', () => { void read(enabled.checked ? 'enable' : 'disable'); });
    retry.addEventListener('click', () => { void read('ensure'); });
    section.querySelectorAll<HTMLButtonElement>('[data-cli-copy]').forEach(button => {
        button.addEventListener('click', async () => {
            const feedback = section.querySelector<HTMLElement>('#cli-copy-status')!;
            try {
                const text = section.querySelector<HTMLElement>(`#${button.dataset.cliCopy}`)!.textContent || '';
                if (!await ClipboardSetText(text)) throw Error('Clipboard unavailable');
                feedback.textContent = 'Copied.';
            } catch {
                feedback.textContent = 'Could not copy. Select and copy the text above.';
            }
        });
    });
    window.addEventListener('focus', () => { void read(); });
    void read();
    return read;
}
