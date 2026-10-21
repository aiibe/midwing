import * as app from '../wailsjs/go/main/App';

export function setupActivity(message: (text: string, error?: boolean) => void) {
    let historyRequest = 0;
    async function history() {
        const request = ++historyRequest;
        try {
            const {entries: rows, warning} = await app.Activity();
            if (request !== historyRequest) return;
            const notice = document.querySelector<HTMLElement>('#history-warning')!;
            notice.textContent = warning || '';
            notice.hidden = !warning;
            const container = document.querySelector('#history')!;
            container.replaceChildren();
            if (!rows.length) {
                container.textContent = warning ? 'No readable activity entries.' : 'No activity yet. Run an API command to see it here.';
                return;
            }
            for (const row of rows.reverse()) {
                const el = document.createElement('div'); el.className = 'history-row';
                for (const value of [new Date(row.time).toLocaleString(), row.operation, row.outcome + (row.status ? ` · ${row.status}` : '')]) {
                    const span = document.createElement('span'); span.textContent = value; el.append(span);
                }
                container.append(el);
            }
        } catch (e) { if (request === historyRequest) message(String(e), true); }
    }

    document.querySelector('#refresh')!.addEventListener('click', history);
    return history;
}
