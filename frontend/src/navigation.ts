export const pages = ['connections', 'activity', 'cli-setup'];

export function setupNavigation(showTab: (tab: string) => void, addService: () => void, isBusy: () => boolean) {
    document.addEventListener('keydown', event => {
        if (event.defaultPrevented || event.isComposing || event.shiftKey || event.altKey || !(event.metaKey || event.ctrlKey) || document.querySelector('dialog[open]')) return;
        const tab = pages[Number(event.key) - 1];
        if (tab) {
            event.preventDefault(); showTab(tab);
            const heading = document.querySelector<HTMLElement>(`#${tab} h1`)!;
            heading.tabIndex = -1; heading.focus({preventScroll: true});
        }
        if (isBusy()) return;
        if (event.key.toLowerCase() === 'n') { event.preventDefault(); addService(); }
        if (event.key.toLowerCase() === 'f') {
            event.preventDefault(); showTab('connections');
            document.querySelector<HTMLInputElement>('#service-search')!.focus();
        }
    });
}
