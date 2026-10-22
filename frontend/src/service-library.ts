import type { midwing } from '../wailsjs/go/models';
type Service = midwing.DesktopService;

export function setupServiceLibrary(select: (id: string) => void) {
    const search = document.querySelector<HTMLInputElement>('#service-search')!;
    const list = document.querySelector('#services')!;
    function filter() {
        const query = search.value.trim().toLocaleLowerCase();
        let visible = 0;
        list.querySelectorAll<HTMLButtonElement>('button').forEach(button => {
            button.hidden = !button.dataset.search!.includes(query);
            if (!button.hidden) visible++;
        });
        const empty = document.querySelector<HTMLElement>('#services-empty')!;
        empty.textContent = query ? 'No matching services.' : 'Your services will appear here.';
        empty.hidden = visible > 0;
    }
    search.addEventListener('input', filter);
    search.addEventListener('keydown', event => {
        if (event.key === 'Escape') { search.value = ''; filter(); }
        if (event.key === 'ArrowDown') { event.preventDefault(); list.querySelector<HTMLButtonElement>('button:not([hidden]):not(:disabled)')?.focus(); }
    });
    list.addEventListener('keydown', event => {
        if (!(event instanceof KeyboardEvent) || !['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
        const buttons = [...list.querySelectorAll<HTMLButtonElement>('button:not([hidden]):not(:disabled)')];
        const index = buttons.indexOf(document.activeElement as HTMLButtonElement);
        const next = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1 : Math.max(0, Math.min(buttons.length - 1, index + (event.key === 'ArrowDown' ? 1 : -1)));
        event.preventDefault(); buttons[next]?.focus();
    });
    return (services: Service[], connections: midwing.DesktopConnection[], selected: string, busy: boolean) => {
        const connected = new Set(connections.map(connection => connection.service));
        const connectedCount = services.filter(item => connected.has(item.id)).length;
        document.querySelector('#connection-count')!.textContent = `${connectedCount} connected`;
        document.querySelector('.connection-summary')!.classList.toggle('has-connections', connectedCount > 0);
        document.querySelector('#nav-count')!.textContent = String(services.length);
        document.querySelector('#service-count')!.textContent = String(services.length);
        const focusedId = list.contains(document.activeElement) ? (document.activeElement as HTMLElement).dataset.service : undefined;
        const {scrollTop, scrollLeft} = list;
        list.replaceChildren();
        for (const service of services) {
            const isConnected = connected.has(service.id);
            const connectionStatus = isConnected ? 'Connected' : 'Not connected';
            const button = document.createElement('button'); button.type = 'button';
            button.className = 'secondary' + (service.id === selected ? ' active-service' : '');
            button.setAttribute('aria-pressed', String(service.id === selected));
            button.setAttribute('aria-label', `${service.name}, ${connectionStatus.toLowerCase()}`);
            button.dataset.service = service.id;
            button.dataset.search = `${service.name} ${service.id} ${service.origin}`.toLocaleLowerCase();
            const info = document.createElement('span'); info.className = 'service-info';
            const name = document.createElement('span'); name.className = 'service-title'; name.textContent = service.name;
            const status = document.createElement('span'); status.className = 'service-status' + (isConnected ? ' is-connected' : ''); status.textContent = connectionStatus;
            info.append(name, status); button.append(info); button.disabled = busy;
            button.addEventListener('click', () => { select(service.id); list.querySelector<HTMLButtonElement>('.active-service')?.focus({preventScroll: true}); });
            list.append(button);
        }
        filter();
        const buttons = [...list.querySelectorAll<HTMLButtonElement>('button:not([hidden])')];
        buttons.find(button => button.dataset.service === focusedId)?.focus({preventScroll: true});
        list.scrollTop = scrollTop;
        list.scrollLeft = scrollLeft;
    };
}
