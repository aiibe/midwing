import { setupUpdates } from './updates';
import template from './template.html?raw';
import { createSettingsRefresh } from './settings-refresh';
import { setupCustomServiceForm } from './custom-service-form';
import { setupActivity } from './activity';
import { setupAgentPrompt } from './agent-setup';
import './style.css';
import { EventsOn } from '../wailsjs/runtime/runtime';
import * as app from '../wailsjs/go/main/App';
import { midwing } from '../wailsjs/go/models';
type Service = midwing.DesktopService;
type Connection = midwing.DesktopConnection;

const root = document.querySelector<HTMLDivElement>('#app')!;
root.innerHTML = template;
const desktopAvailable = () => !!(window as Window & {go?: unknown}).go;
let savedChange = '';
let savedWarning = '';
const changeMessage = () => savedChange + (savedWarning ? ` ${savedWarning}` : '');
const message = (text: string, error = false, preserveChange = false) => {
    if (!preserveChange) { savedChange = ''; savedWarning = ''; }
    const el = document.querySelector<HTMLDivElement>('#message')!;
    el.textContent = text; el.className = error ? 'error' : 'success';
    const connectionsTab = document.querySelector<HTMLElement>('#connections')!;
    if (!connectionsTab.hidden) {
        const customForm = document.querySelector<HTMLElement>('#custom')!;
        if (!customForm.hidden) customForm.append(el);
        else if (!services.length) document.querySelector('.service-detail')!.append(el);
        else document.querySelector('#service-content .connection')!.insertAdjacentElement('afterend', el);
    } else {
        document.querySelector<HTMLElement>('main > section:not([hidden])')?.append(el);
    }
};
let services: Service[] = [];
let connections: Connection[] = [];
let selected = '';
let busy = false;
let permissionsDirty = false;
let reconnecting = false;
document.querySelector('#permissions')!.addEventListener('change', event => { if ((event.target as HTMLInputElement).name === 'permission') { permissionsDirty = true; updatePermissionCount(); } });
const descriptionDrafts = new Map<string, string>();
const current = () => services.find(s => s.id === selected)!;
const currentConnection = () => connections.find(connection => connection.service === selected);
const serviceActions = document.querySelector<HTMLDetailsElement>('#service-actions')!;
const deleteDialog = document.querySelector<HTMLDialogElement>('#delete-confirm')!;
function updatePermissionCount() {
    const checks = [...document.querySelectorAll<HTMLInputElement>('[name="permission"]')];
    document.querySelector('#permission-count')!.textContent = `${checks.filter(check => check.checked).length} of ${checks.length} enabled`;
}
function syncCredentialFields(clear = false) {
    const service = current();
    if (!service) return;
    const password = service.authKind === 'password';
    const connected = !!currentConnection();
    const editing = !connected || reconnecting;
    document.querySelector<HTMLDivElement>('#password-fields')!.hidden = !password || !editing;
    document.querySelector<HTMLDivElement>('#token-fields')!.hidden = password || !editing;
    for (const id of ['token', 'email', 'password']) {
        const input = document.querySelector<HTMLInputElement>(`#${id}`)!;
        const active = editing && (id === 'token' ? !password : password);
        if (clear) input.value = '';
        input.required = active;
        input.disabled = busy || !active;
    }
    document.querySelector<HTMLButtonElement>('#reconnect')!.hidden = !connected || reconnecting;
    document.querySelector<HTMLButtonElement>('#cancel-reconnect')!.hidden = !connected || !reconnecting;
    document.querySelector<HTMLButtonElement>('#save-connection')!.hidden = !editing;
    document.querySelector('#save-connection')!.textContent = connected ? 'Reconnect' : 'Save connection';
    document.querySelector('#permissions')!.classList.toggle('permissions-readonly', !editing);
    document.querySelectorAll<HTMLInputElement>('[name="permission"]').forEach(input => { input.disabled = busy || !editing; });
}
function renderService(preserveEdits = false) {
    if (!preserveEdits) { permissionsDirty = false; reconnecting = false; }
    const service = current();
    renderServiceLibrary();
    syncServiceVisibility();
    deleteDialog.close();
    if (!service) return;
    const conn = currentConnection();
    document.querySelector<HTMLButtonElement>('#delete-service')!.hidden = !service.custom;
    document.querySelector('#service-name')!.textContent = service.name;
    document.querySelector('#service-description')!.textContent = service.description;
    document.querySelector('#service-origin')!.textContent = service.origin;
    document.querySelector('#badge')!.textContent = conn ? 'Connected' : 'Not connected';
    document.querySelector('#badge')!.classList.toggle('connected', !!conn);
    document.querySelector<HTMLButtonElement>('#remove')!.hidden = !conn;
    serviceActions.hidden = !conn && !service.custom;
    serviceActions.open = false;
    const checks = new Map([...document.querySelectorAll<HTMLInputElement>('[name="permission"]')].map(el => [el.value, el.checked]));
    const permissions = document.querySelector('#permissions')!;
    permissions.replaceChildren();
    for (const permission of service.permissions) {
        const label = document.createElement('label'); label.className = 'check';
        const checkbox = document.createElement('input'); checkbox.type = 'checkbox';
        checkbox.name = 'permission'; checkbox.value = permission.id;
        checkbox.checked = preserveEdits && permissionsDirty && checks.has(permission.id) ? checks.get(permission.id)! : conn ? conn.permissions.includes(permission.id) : true;
        const details = document.createElement('div'); details.textContent = permission.label;
        // Custom endpoints already contain the path in their label.
        if (permission.description !== permission.label.replace(/^GET /, 'Read ')) {
            const description = document.createElement('small'); description.textContent = permission.description;
            details.append(description);
        }
        label.append(checkbox, details);
        const row = document.createElement('div'); row.className = 'permission-row';
        row.append(label);
        if (service.custom) addInlineDescription(row, service, permission.label.replace(/^GET /, ''), permission.description);
        permissions.append(row);
    }
    syncCredentialFields(!preserveEdits);
    updatePermissionCount();
}
function syncServiceVisibility(custom = !document.getElementById('custom')!.hidden) {
    document.getElementById('service-content')!.hidden = custom || !services.length;
    document.getElementById('empty-services')!.hidden = custom || !!services.length;
}
function renderServiceLibrary() {
    const connectedServices = new Set(connections.map(connection => connection.service));
    document.querySelector('#connection-count')!.textContent = `${services.filter(item => connectedServices.has(item.id)).length} connected`;
    document.querySelector('#service-count')!.textContent = String(services.length);
    const tabs = document.querySelector('#services')!; tabs.replaceChildren();
    for (const item of services) {
        const button = document.createElement('button'); button.type = 'button';
        button.className = 'secondary' + (item.id === selected ? ' active-service' : '');
        button.setAttribute('aria-pressed', String(item.id === selected));
        const connected = connectedServices.has(item.id);
        button.setAttribute('aria-label', `${item.name}, ${connected ? 'connected' : 'not connected'}`);
        const info = document.createElement('span'); info.className = 'service-info';
        const name = document.createElement('span'); name.className = 'service-title'; name.textContent = item.name;
        const status = document.createElement('span'); status.className = 'service-status' + (connected ? ' is-connected' : ''); status.textContent = connected ? 'Connected' : 'Not connected';
        info.append(name, status); button.append(info); button.disabled = busy;
        button.addEventListener('click', () => { selected = item.id; showTab('connections', false); renderService(); });
        tabs.append(button);
    }
}
async function readSettings(background: boolean, preserveEdits: boolean) {
    if (!desktopAvailable()) { message('Open the desktop app to manage connections.', true); return; }
    const next = await app.State();
    if (background && busy) { settingsRefresh.request(); return; }
    if (background && savedChange) message(changeMessage(), !!savedWarning, true);
    if (background && JSON.stringify(next) === JSON.stringify({services, connections})) return;
    const previous = current();
    services = next.services; connections = next.connections;
    if (!services.some(service => service.id === selected)) selected = services[0]?.id || '';
    const service = current();
    const sameDefinition = previous && service && previous.id === service.id && previous.revision === service.revision;
    renderService(!!(preserveEdits && sameDefinition));
}
const settingsRefresh = createSettingsRefresh(
    readSettings,
    () => busy,
    () => savedChange
        ? message(`${changeMessage()} Could not refresh the view. Refocus the app to retry.`, true, true)
        : message('Could not refresh settings. Your last valid view is retained; refocus the app to retry.', true),
);
const load = settingsRefresh.load;
const refreshSettings = settingsRefresh.request;
const stopSettingsListener = (window as Window & {runtime?: unknown}).runtime ? EventsOn('settings-changed', refreshSettings) : undefined;
window.addEventListener('focus', refreshSettings);
document.addEventListener('visibilitychange', () => { if (!document.hidden) refreshSettings(); });
window.addEventListener('beforeunload', () => stopSettingsListener?.());
function setBusy(value: boolean) {
    busy = value;
    if (!value) settingsRefresh.resume();
    document.querySelectorAll<HTMLButtonElement | HTMLInputElement>('#connections button, #connections input').forEach(el => el.disabled = value);
    syncCredentialFields();
}
async function reloadAfterChange(success: string, warning = '', preserveEdits = false): Promise<boolean> {
    savedChange = success;
    savedWarning = warning;
    try {
        await load(false, preserveEdits);
        message(changeMessage(), !!savedWarning, true);
        return true;
    } catch {
        message(`${changeMessage()} Could not refresh the view. Refocus the app to retry.`, true, true);
        return false;
    }
}
async function mutateConnection(action: () => Promise<midwing.ChangeResult>, success: string, pending = '') {
    if (busy) return;
    setBusy(true); message(pending);
    try {
        const result = await action();
        await reloadAfterChange(success, result?.warning);
    } catch (e) { message(String(e), true); }
    finally { setBusy(false); }
}
document.querySelector('#form')!.addEventListener('submit', async e => {
    e.preventDefault(); if (busy || !current()) return;
    const service = current();
    const tokenInput = document.querySelector<HTMLInputElement>('#token')!;
    const passwordInput = document.querySelector<HTMLInputElement>('#password')!;
    const emailInput = document.querySelector<HTMLInputElement>('#email')!;
    const token = tokenInput.value.trim(); const password = passwordInput.value;
    const email = emailInput.value.trim();
    tokenInput.value = ''; passwordInput.value = '';
    const permissions = [...document.querySelectorAll<HTMLInputElement>('[name="permission"]:checked')].map(el => el.value);
    await mutateConnection(
        () => service.authKind === 'password'
            ? app.ConnectPassword(service.id, email, password, permissions)
            : app.SaveConnection(service.id, token, permissions),
        `${service.name} connected. Your tools can now call its API.`,
        service.authKind === 'password' ? 'Signing in…' : 'Saving connection…',
    );
});
document.addEventListener('pointerdown', event => {
    if (event.target instanceof Node && !serviceActions.contains(event.target)) serviceActions.open = false;
});
document.addEventListener('keydown', event => {
    if (event.key === 'Escape' && serviceActions.open) {
        serviceActions.open = false;
        serviceActions.querySelector<HTMLElement>('summary')!.focus();
    }
});
document.querySelector('#reconnect')!.addEventListener('click', () => {
    reconnecting = true;
    syncCredentialFields(true);
    document.querySelector<HTMLInputElement>(current().authKind === 'password' ? '#email' : '#token')!.focus();
});
document.querySelector('#cancel-reconnect')!.addEventListener('click', () => renderService());
document.querySelector('#remove')!.addEventListener('click', async () => {
    const service = current();
    serviceActions.open = false;
    await mutateConnection(() => app.RemoveConnection(service.id), `${service.name} disconnected.`);
});

document.querySelector('#delete-service')!.addEventListener('click', () => {
    serviceActions.open = false;
    document.querySelector('#delete-title')!.textContent = `Delete ${current().name}?`;
    deleteDialog.showModal();
});
document.querySelector('#cancel-delete')!.addEventListener('click', () => deleteDialog.close());
document.querySelector('#confirm-delete')!.addEventListener('click', async () => {
    const service = current();
    deleteDialog.close();
    await mutateConnection(() => app.DeleteCustomService(service.id), `${service.name} deleted.`);
});
function showTab(tab: string, refresh = true) {
    const activeTab = tab === 'custom' ? 'connections' : tab;
    for (const id of ['connections', 'activity', 'agent']) document.getElementById(id)!.hidden = id !== activeTab;
    document.getElementById('custom')!.hidden = tab !== 'custom';
    syncServiceVisibility(tab === 'custom');
    document.querySelectorAll<HTMLButtonElement>('[data-tab]').forEach(el => el.classList.toggle('selected', el.dataset.tab === activeTab));
    message('');
    if (tab === 'connections' && refresh) refreshSettings();
    if (tab === 'activity') void history();
    if (tab === 'agent') void refreshPrompt();
}
const openCustomForm = setupCustomServiceForm({
    isBusy: () => busy,
    setBusy,
    selectService: id => { selected = id; },
    showTab,
    reloadAfterChange,
});
const history = setupActivity(message);
const refreshPrompt = setupAgentPrompt(message);

document.querySelector('#add-service')!.addEventListener('click', openCustomForm);
document.querySelectorAll<HTMLButtonElement>('[data-tab]').forEach(button => button.addEventListener('click', () => showTab(button.dataset.tab!)));
void load().catch(e => message(String(e), true));

function addInlineDescription(row: HTMLDivElement, service: Service, path: string, description: string) {
    const key = `${service.id}:${path}`;
    const edit = document.createElement('button'); edit.type = 'button'; edit.className = 'permission-edit';
    edit.textContent = 'Edit'; edit.setAttribute('aria-label', `Edit description for GET ${path}`);
    const editor = document.createElement('div'); editor.className = 'permission-editor';
    const input = document.createElement('input'); input.className = 'permission-description'; input.maxLength = 500;
    input.placeholder = `Read ${path}`;
    input.value = descriptionDrafts.get(key) ?? (description === input.placeholder ? '' : description);
    input.setAttribute('aria-label', `Description for GET ${path}`);
    const actions = document.createElement('div'); actions.className = 'permission-editor-actions';
    const save = document.createElement('button'); save.type = 'button'; save.className = 'primary'; save.textContent = 'Save';
    const cancel = document.createElement('button'); cancel.type = 'button'; cancel.className = 'secondary'; cancel.textContent = 'Cancel';
    const error = document.createElement('p'); error.className = 'error'; error.setAttribute('role', 'alert');
    actions.append(save, cancel); editor.append(input, actions, error);
    const setEditing = (editing: boolean) => {
        editor.hidden = !editing; edit.hidden = editing;
        edit.setAttribute('aria-expanded', String(editing));
        error.textContent = '';
    };
    setEditing(descriptionDrafts.has(key));
    edit.addEventListener('click', () => {
        if (busy) return;
        descriptionDrafts.set(key, input.value);
        setEditing(true); input.focus();
    });
    input.addEventListener('input', () => descriptionDrafts.set(key, input.value));
    cancel.addEventListener('click', () => {
        if (busy) return;
        descriptionDrafts.delete(key);
        input.value = description === input.placeholder ? '' : description;
        setEditing(false); edit.focus();
    });
    save.addEventListener('click', async () => {
        if (busy) return;
        setBusy(true); save.textContent = 'Saving…'; error.textContent = '';
        try {
            const result = await app.UpdatePermissionDescriptions(service.id, {[path]: input.value.trim()});
            descriptionDrafts.delete(key);
            await reloadAfterChange('Permission description saved.', result?.warning, true);
            // If refreshing failed, keep the local row usable for another edit.
            description = input.value.trim() || input.placeholder;
            setEditing(false);
        } catch (failure) { error.textContent = String(failure); }
        finally { save.textContent = 'Save'; setBusy(false); }
    });
    input.addEventListener('keydown', event => {
        if (event.key === 'Enter') { event.preventDefault(); save.click(); }
        if (event.key === 'Escape') { event.preventDefault(); cancel.click(); }
    });
    row.append(edit, editor);
}

setupUpdates();
