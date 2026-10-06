import * as app from '../wailsjs/go/main/App';

export function setupAgentPrompt(message: (text: string, error?: boolean) => void) {
    async function refreshPrompt() {
        const button = document.querySelector<HTMLButtonElement>('#copy-prompt')!;
        button.disabled = true;
        document.querySelector('#copy-status')!.textContent = '';
        try {
            document.querySelector<HTMLTextAreaElement>('#agent-prompt')!.value = await app.AgentPrompt();
            button.disabled = false;
        } catch (e) { message(String(e), true); }
    }

    document.querySelector('#copy-prompt')!.addEventListener('click', async () => {
        const button = document.querySelector<HTMLButtonElement>('#copy-prompt')!;
        button.disabled = true;
        try {
            await app.CopyAgentPrompt();
            document.querySelector('#copy-status')!.textContent = 'Copied. Paste into your coding agent.';
        } catch (e) {
            const text = document.querySelector<HTMLTextAreaElement>('#agent-prompt')!;
            text.focus(); text.select();
            document.querySelector('#copy-status')!.textContent = 'Clipboard unavailable. The prompt is selected; press ⌘C or Ctrl+C to copy it.';
        } finally { button.disabled = false; }
    });

    return refreshPrompt;
}
