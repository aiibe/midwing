import * as app from '../wailsjs/go/main/App';

export function setupCLI() {
    const section = document.querySelector<HTMLElement>('#cli-setup')!;
    const status = section.querySelector<HTMLElement>('#cli-status')!;
    const warning = section.querySelector<HTMLElement>('#cli-warning')!;
    const destination = section.querySelector<HTMLElement>('#cli-destination')!;
    const target = section.querySelector<HTMLElement>('#cli-target')!;
    const path = section.querySelector<HTMLElement>('#cli-path')!;
    const error = section.querySelector<HTMLElement>('#cli-error')!;
    const buttons = [...section.querySelectorAll<HTMLButtonElement>('[data-cli-action]')];
    const notice = document.querySelector<HTMLButtonElement>('#cli-notice')!;
    let busy = false;
    async function read(action?: string) {
        if (busy || !('go' in window)) return;
        busy = true;
        buttons.forEach(button => { button.disabled = true; });
        error.textContent = '';
        try {
            const next = action ? await app.ChangeCLISetup(action) : await app.CLISetupStatus();
            section.querySelector<HTMLElement>('#cli-details')!.hidden = !next.available;
            status.textContent = next.message;
            section.querySelector<HTMLElement>('#cli-change-explanation')!.hidden = !next.canRemove;
            warning.textContent = next.warning;
            warning.hidden = !next.warning;
            destination.textContent = next.destination;
            section.querySelector<HTMLElement>('#cli-existing-row')!.hidden = !next.existingTarget;
            section.querySelector<HTMLElement>('#cli-existing')!.textContent = next.existingTarget;
            target.textContent = next.target;
            path.textContent = next.pathCommand;
            const allowed: Record<string, boolean> = {install: next.canInstall, repair: next.canRepair, remove: next.canRemove};
            buttons.forEach(button => { button.hidden = !allowed[button.dataset.cliAction!]; });
            notice.hidden = !next.available || (next.installed && !next.canRepair && !next.warning);
            notice.textContent = next.canRepair ? 'Repair CLI setup' : next.installed ? 'Review CLI setup' : 'Set up the CLI';
        } catch (e) {
            error.textContent = `Could not ${action || 'read'} CLI setup: ${String(e)}. Refresh to retry.`;
        } finally {
            busy = false;
            buttons.forEach(button => { button.disabled = false; });
        }
    }
    buttons.forEach(button => button.addEventListener('click', () => { void read(button.dataset.cliAction); }));
    section.querySelector('#cli-refresh')!.addEventListener('click', () => { void read(); });
    // The PATH check must happen in the user's terminal, not the GUI environment.
    window.addEventListener('focus', () => { void read(); });
    void read();
}
