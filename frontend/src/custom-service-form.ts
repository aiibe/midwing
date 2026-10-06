import * as app from '../wailsjs/go/main/App';
import { midwing } from '../wailsjs/go/models';

interface CustomServiceFormOptions {
    isBusy: () => boolean;
    setBusy: (value: boolean) => void;
    selectService: (id: string) => void;
    showTab: (tab: string, refresh?: boolean) => void;
    reloadAfterChange: (success: string) => Promise<boolean>;
}

export function setupCustomServiceForm({isBusy, setBusy, selectService, showTab, reloadAfterChange}: CustomServiceFormOptions) {
    const customValue = (id: string) => document.querySelector<HTMLInputElement>(`#custom-${id}`)!.value.trim();
    let idEdited = false;
    function syncCustomAuth() {
        const password = document.querySelector<HTMLSelectElement>('#custom-auth')!.value === 'password';
        document.querySelector<HTMLDivElement>('#custom-login')!.hidden = !password;
        document.querySelectorAll<HTMLInputElement>('#custom-login input').forEach(input => { input.disabled = !password; input.required = password && input.id !== 'custom-refresh-path'; });
    }
    function addEndpointRow() {
        const row = document.createElement('div'); row.className = 'endpoint-row';
        row.innerHTML = `<div class="endpoint-path-field"><label>GET path<input class="endpoint-path" placeholder="/items or /items/{id}" required></label></div><button type="button" class="danger" aria-label="Remove endpoint">Remove</button><label class="endpoint-options">Permission description<input class="endpoint-description" maxlength="500" placeholder="Optional, e.g. Read your invoices"></label><details class="endpoint-options"><summary>Query parameters</summary><label>Allowed names<input class="endpoint-query" placeholder="page, limit, filter"></label></details>`;
        row.querySelector('button')!.addEventListener('click', () => row.remove());
        document.querySelector('#custom-endpoints')!.append(row);
    }
    function openCustomForm() {
        document.querySelector<HTMLFormElement>('#custom-form')!.reset();
        idEdited = false;
        document.querySelectorAll<HTMLDetailsElement>('#custom-form details').forEach(details => details.open = false);
        document.querySelector('#custom-error')!.textContent = '';
        document.querySelector('#custom-endpoints')!.replaceChildren(); addEndpointRow(); syncCustomAuth();
        showTab('custom'); document.querySelector<HTMLInputElement>('#custom-name')!.focus();
    }
    document.querySelector('#custom-name')!.addEventListener('input', () => {
        if (!idEdited) document.querySelector<HTMLInputElement>('#custom-id')!.value = customValue('name').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 48);
    });
    document.querySelector('#custom-id')!.addEventListener('input', () => { idEdited = true; });
    document.querySelector('#custom-auth')!.addEventListener('change', syncCustomAuth);
    document.querySelector('#add-endpoint')!.addEventListener('click', addEndpointRow);
    document.querySelector('#cancel-custom')!.addEventListener('click', () => showTab('connections'));
    // Reveal invalid fields before the browser tries to focus a collapsed input.
    document.querySelector('#custom-form')!.addEventListener('invalid', event => {
        if (event.target instanceof HTMLElement) {
            let details = event.target.closest('details');
            while (details) { details.open = true; details = details.parentElement?.closest('details') ?? null; }
        }
    }, true);
    document.querySelector('#custom-form')!.addEventListener('submit', async e => {
        e.preventDefault();
        if (isBusy()) return;
        const form = document.querySelector<HTMLFormElement>('#custom-form')!;
        const authKind = document.querySelector<HTMLSelectElement>('#custom-auth')!.value as 'token' | 'password';
        const prefix = customValue('prefix');
        const input = {
            id: customValue('id'), name: customValue('name'), baseUrl: customValue('url'), authKind,
            header: customValue('header'), prefix: prefix ? prefix + ' ' : '',
            endpoints: [...document.querySelectorAll<HTMLDivElement>('.endpoint-row')].map(row => ({
                description: row.querySelector<HTMLInputElement>('.endpoint-description')!.value.trim(),
                path: row.querySelector<HTMLInputElement>('.endpoint-path')!.value.trim(),
                queryParameters: row.querySelector<HTMLInputElement>('.endpoint-query')!.value.split(',').map(key => key.trim()).filter(Boolean),
            })),
        } satisfies Omit<midwing.CustomService, 'convertValues'>;
        const definition = new midwing.CustomService(input);
        if (authKind === 'password') definition.password = {loginPath: customValue('login-path'), emailField: customValue('email-field'), passwordField: customValue('password-field'), tokenField: customValue('token-field'), refreshPath: customValue('refresh-path')};
        const controls = form.querySelectorAll<HTMLInputElement | HTMLButtonElement | HTMLSelectElement>('input, button, select');
        controls.forEach(control => control.disabled = true);
        setBusy(true);
        document.querySelector('#custom-error')!.textContent = '';
        try {
            await app.CreateCustomService(definition);
            selectService(definition.id);
            showTab('connections', false);
            if (await reloadAfterChange(`${definition.name} created. Enter your credentials to connect it.`)) {
                document.querySelector<HTMLInputElement>(authKind === 'token' ? '#token' : '#email')!.focus();
            }
        } catch (e) { document.querySelector('#custom-error')!.textContent = String(e); }
        finally { controls.forEach(control => control.disabled = false); syncCustomAuth(); setBusy(false); }
    });

    addEndpointRow();
    syncCustomAuth();
    return openCustomForm;
}
