// Serialize reads and defer background refreshes while a connection is being changed.
export function createSettingsRefresh(
    read: (background: boolean, preserveEdits: boolean) => Promise<void>,
    isBusy: () => boolean,
    onError: () => void,
) {
    let loading: Promise<void> | undefined;
    let pending = false;

    async function load(background = false, preserveEdits = background): Promise<void> {
        if (background && isBusy()) { pending = true; return; }
        if (loading) {
            // The preceding caller owns its error; this caller still needs a fresh read.
            try { await loading; } catch { /* Continue the queued read. */ }
            return load(background, preserveEdits);
        }
        loading = read(background, preserveEdits);
        try { await loading; } finally { loading = undefined; }
    }

    function request() {
        if (pending) return;
        pending = true;
        queueMicrotask(() => {
            if (isBusy()) return;
            pending = false;
            void load(true).catch(onError);
        });
    }

    function resume() {
        if (pending) { pending = false; request(); }
    }

    return {load, request, resume};
}
