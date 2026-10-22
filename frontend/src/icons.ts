import { createElement, PanelsTopLeft, Activity, SquareTerminal, ShieldCheck, Plus, Search, Ellipsis, RotateCw, CircleAlert } from 'lucide';

const icons = {connections: PanelsTopLeft, activity: Activity, terminal: SquareTerminal, shield: ShieldCheck, plus: Plus, search: Search, more: Ellipsis, refresh: RotateCw, warning: CircleAlert};
export function icon(name: keyof typeof icons) {
    return createElement(icons[name], {width: 18, height: 18, 'stroke-width': 1.7, 'aria-hidden': 'true', focusable: 'false'});
}
export function setupIcons() {
    document.querySelectorAll<HTMLElement>('[data-icon]').forEach(element => {
        const name = element.dataset.icon as keyof typeof icons;
        if (name in icons) element.replaceChildren(icon(name));
    });
}
