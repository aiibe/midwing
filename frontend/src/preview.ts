// Development-only, in-memory sample accounts. Never reads or writes real settings.
export function setupPreview() {
    const permission = (id: string, label: string, description: string) => ({id, label, description});
    const service = (id: string, name: string, description: string, origin: string, permissions: ReturnType<typeof permission>[]) => ({id, name, description, origin, permissions, authKind: 'token', examples: [], custom: true, revision: 'preview'});
    let services = [
        service('github', 'GitHub', 'Repositories, issues, and profile.', 'https://api.github.com', [permission('profile', 'GET /user', 'Read your profile and account information'), permission('repos', 'GET /user/repos', 'Browse your repositories'), permission('issues', 'GET /repos/{owner}/{repo}/issues', 'Read issues in a repository'), permission('create-issue', 'POST /repos/{owner}/{repo}/issues', 'Create issues on your behalf')]),
        service('linear', 'Linear', 'Projects and issues.', 'https://api.linear.app', [permission('issues', 'GET /issues', 'Read issues and project updates'), permission('projects', 'GET /projects', 'Browse your workspace projects')]),
        service('notion', 'Notion', 'Pages and workspace search.', 'https://api.notion.com/v1', [permission('pages', 'GET /pages/{id}', 'Read pages in your workspace'), permission('search', 'POST /search', 'Search your connected pages')]),
        service('acme', 'Acme API', 'Internal API.', 'https://api.example.com/v1', [permission('items', 'GET /items', 'Browse items in your workspace'), permission('item', 'GET /items/{id}', 'Read an item’s details')]),
    ];
    let connections = [{service: 'github', permissions: ['profile', 'repos', 'issues']}, {service: 'linear', permissions: ['issues', 'projects']}];
    let enabled = true;
    const status = () => ({available: true, installed: enabled, autoSetupDisabled: !enabled, setupError: '', destination: '~/.local/bin/midwing', target: '/Applications/Midwing.app', existingTarget: '', pathCommand: 'export PATH="$HOME/.local/bin:$PATH"', message: enabled ? 'CLI installed.' : 'Enable CLI to make the command available.'});
    const save = async (id: string, _revision: string, _token: string, permissions: string[]) => {
        connections = [...connections.filter(connection => connection.service !== id), {service: id, permissions}];
        return {warning: ''};
    };
    const mock = {
        State: async () => structuredClone({services, connections}),
        Version: async () => '',
        CheckForUpdate: async () => ({version: '', downloadURL: ''}),
        CLISetupStatus: async () => status(),
        ChangeCLISetup: async (action: string) => { enabled = action !== 'disable'; return status(); },
        SaveConnection: save,
        ConnectPassword: async (id: string, revision: string, _email: string, password: string, permissions: string[]) => save(id, revision, password, permissions),
        RemoveConnection: async (id: string) => { connections = connections.filter(connection => connection.service !== id); return {warning: ''}; },
        DeleteCustomService: async (id: string) => { services = services.filter(service => service.id !== id); connections = connections.filter(connection => connection.service !== id); return {warning: ''}; },
        UpdatePermissionDescriptions: async (id: string, descriptions: Record<string, string>) => {
            const selected = services.find(service => service.id === id)!;
            selected.permissions.forEach(permission => { const path = permission.label.replace(/^(GET|POST) /, ''); if (path in descriptions) permission.description = descriptions[path]; });
            return {warning: ''};
        },
        CreateCustomService: async (definition: {id: string; name: string; baseUrl: string; authKind: string; endpoints: {path: string; methods: string[]; description: string}[]}) => {
            if (services.some(service => service.id === definition.id)) throw Error('A service with this ID already exists.');
            services.push({...service(definition.id, definition.name, '', definition.baseUrl, definition.endpoints.flatMap(endpoint => endpoint.methods.map(method => permission(`${method}:${endpoint.path}`, `${method} ${endpoint.path}`, endpoint.description)))), authKind: definition.authKind});
        },
        Activity: async () => ({entries: [
            {time: new Date(Date.now() - 240000).toISOString(), operation: 'GET github /user/repos', outcome: 'success', status: 200},
            {time: new Date(Date.now() - 180000).toISOString(), operation: 'GET linear /issues', outcome: 'success', status: 200},
            {time: new Date(Date.now() - 120000).toISOString(), operation: 'POST github /repos/acme/app/issues', outcome: 'denied', status: 0},
            {time: new Date(Date.now() - 60000).toISOString(), operation: 'GET github /user', outcome: 'success', status: 200},
        ], warning: ''}),
    };
    Object.assign(window, {go: {main: {App: mock}}, runtime: {EventsOnMultiple: () => () => {}, ClipboardSetText: (text: string) => navigator.clipboard.writeText(text).then(() => true)}});
}
