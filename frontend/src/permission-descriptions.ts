import * as app from '../wailsjs/go/main/App';
import { midwing } from '../wailsjs/go/models';

type Service = midwing.DesktopService;

interface PermissionDescriptionOptions {
    isBusy: () => boolean;
    setBusy: (value: boolean) => void;
    reloadAfterChange: (success: string, warning?: string, preserveEdits?: boolean) => Promise<boolean>;
}

export const permissionPath = (label: string) => label.replace(/^(GET|POST) /, '');
export const defaultPermissionDescription = (label: string) => label.startsWith('POST ')
    ? `Submit a request to ${permissionPath(label)}`
    : label.replace(/^GET /, 'Read ');
export function setupPermissionDescriptions({isBusy, setBusy, reloadAfterChange}: PermissionDescriptionOptions) {
    const descriptionDrafts = new Map<string, string>();
    function addInlineDescription(row: HTMLDivElement, service: Service, label: string, description: string, shared: boolean) {
        const path = permissionPath(label);
        const key = `${service.id}:${path}`;
        const edit = document.createElement('button'); edit.type = 'button'; edit.className = 'permission-edit';
        edit.textContent = 'Edit'; edit.setAttribute('aria-label', `Edit description for ${label}`);
        const editor = document.createElement('div'); editor.className = 'permission-editor';
        const input = document.createElement('input'); input.className = 'permission-description'; input.maxLength = 500;
        input.placeholder = defaultPermissionDescription(label);
        input.value = descriptionDrafts.get(key) ?? (description === input.placeholder ? '' : description);
        input.setAttribute('aria-label', `Description for ${label}`);
        const actions = document.createElement('div'); actions.className = 'permission-editor-actions';
        const save = document.createElement('button'); save.type = 'button'; save.className = 'primary'; save.textContent = 'Save';
        const cancel = document.createElement('button'); cancel.type = 'button'; cancel.className = 'secondary'; cancel.textContent = 'Cancel';
        const error = document.createElement('p'); error.className = 'error'; error.setAttribute('role', 'alert');
        actions.append(save, cancel); editor.append(input, actions, error);
        if (shared) {
            const hint = document.createElement('p');
            hint.textContent = 'This description applies to GET and POST for this path.';
            editor.prepend(hint);
            edit.setAttribute('aria-label', `Edit description for GET and POST ${path}`);
        }
        const setEditing = (editing: boolean) => {
            editor.hidden = !editing; edit.hidden = editing;
            edit.setAttribute('aria-expanded', String(editing));
            error.textContent = '';
        };
        setEditing(descriptionDrafts.has(key));
        edit.addEventListener('click', () => {
            if (isBusy()) return;
            descriptionDrafts.set(key, input.value);
            setEditing(true); input.focus();
        });
        input.addEventListener('input', () => descriptionDrafts.set(key, input.value));
        cancel.addEventListener('click', () => {
            if (isBusy()) return;
            descriptionDrafts.delete(key);
            input.value = description === input.placeholder ? '' : description;
            setEditing(false); edit.focus();
        });
        save.addEventListener('click', async () => {
            if (isBusy()) return;
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

    return addInlineDescription;
}
