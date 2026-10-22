import { icon } from './icons';
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
                const empty = document.createElement('div'); empty.className = 'empty-state';
                const title = document.createElement('h3'); title.textContent = warning ? 'No readable activity entries.' : 'No activity yet';
                const hint = document.createElement('p'); hint.textContent = 'Run an API command to see it here.';
                empty.append(icon('activity'), title, hint); container.append(empty);
                return;
            }
            for (const row of [...rows].reverse()) {
                const el = document.createElement('div'); el.className = 'history-row';
                for (const value of [new Date(row.time).toLocaleString(), row.operation, row.outcome + (row.status ? ` · ${row.status}` : '')]) {
                    const span = document.createElement('span'); span.textContent = value; el.append(span);
                }
                const outcome = el.lastElementChild as HTMLElement;
                outcome.className = 'history-outcome' + (/success/i.test(row.outcome) ? ' is-success' : /error|fail|denied/i.test(row.outcome) ? ' is-error' : '');
                container.append(el);
            }
        } catch (e) { if (request === historyRequest) message(String(e), true); }
    }

    document.querySelector('#refresh')!.addEventListener('click', history);
    return history;
}
