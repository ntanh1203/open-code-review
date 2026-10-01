// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

(() => {
    const panel = document.querySelector('.review-launcher');
    if (!panel) return;
    const form = document.getElementById('review-form');
    const message = document.getElementById('review-message');
    const list = document.getElementById('review-jobs');
    const empty = document.getElementById('review-jobs-empty');
    const toggle = document.getElementById('review-language');
    // Escaped so the source stays ASCII (make english-check).
    const vi = {
        'Start a review': 'B\u1eaft \u0111\u1ea7u review',
        'Source folder': 'Th\u01b0 m\u1ee5c source',
        'Base branch': 'Nh\u00e1nh \u0111\u1ed1i chi\u1ebfu',
        'Review branch': 'Nh\u00e1nh c\u1ea7n review',
        'Review jobs': 'C\u00e1c l\u01b0\u1ee3t review',
        'No reviews started yet': 'Ch\u01b0a c\u00f3 l\u01b0\u1ee3t review n\u00e0o',
        queued: '\u0110ang ch\u1edd',
        running: '\u0110ang review',
        completed: 'Ho\u00e0n t\u1ea5t',
        failed: 'Th\u1ea5t b\u1ea1i',
        'Open result': 'Xem k\u1ebft qu\u1ea3',
        'Could not load jobs': 'Kh\u00f4ng t\u1ea3i \u0111\u01b0\u1ee3c danh s\u00e1ch',
        'Review could not start': 'Kh\u00f4ng th\u1ec3 b\u1eaft \u0111\u1ea7u review',
        'Review queued': '\u0110\u00e3 x\u1ebfp h\u00e0ng review',
        'Choose folder': 'Ch\u1ecdn th\u01b0 m\u1ee5c',
        'Could not select folder': 'Kh\u00f4ng th\u1ec3 ch\u1ecdn th\u01b0 m\u1ee5c',
        'Fetch origin': 'T\u1ea3i nh\u00e1nh origin',
        'Could not fetch branches': 'Kh\u00f4ng t\u1ea3i \u0111\u01b0\u1ee3c nh\u00e1nh',
        'Branches loaded': '\u0110\u00e3 t\u1ea3i danh s\u00e1ch nh\u00e1nh',
        'LLM settings': 'C\u1ea5u h\u00ecnh LLM',
        'Base URL': 'Base URL',
        Model: 'Model',
        'Save settings': 'L\u01b0u c\u1ea5u h\u00ecnh',
        'Settings saved': '\u0110\u00e3 l\u01b0u c\u1ea5u h\u00ecnh',
        'Could not save settings': 'Kh\u00f4ng l\u01b0u \u0111\u01b0\u1ee3c c\u1ea5u h\u00ecnh',
        'Could not load settings': 'Kh\u00f4ng t\u1ea3i \u0111\u01b0\u1ee3c c\u1ea5u h\u00ecnh',
        'Saved; leave empty to keep': '\u0110\u00e3 l\u01b0u; \u0111\u1ec3 tr\u1ed1ng \u0111\u1ec3 gi\u1eef nguy\u00ean'
    };
    let language = localStorage.getItem('ocr-viewer-language') === 'vi' ? 'vi' : 'en';
    let jobs = [];
    const text = key => (language === 'vi' && vi[key]) || key;

    const say = (key, detail, isError) => {
        message.textContent = detail ? `${text(key)}: ${detail}` : text(key);
        message.classList.toggle('is-error', Boolean(isError));
    };

    const elapsed = job => {
        const end = job.finished ? new Date(job.finished) : new Date();
        const seconds = Math.max(0, Math.round((end - new Date(job.started)) / 1000));
        const m = Math.floor(seconds / 60);
        return m ? `${m}m ${String(seconds % 60).padStart(2, '0')}s` : `${seconds}s`;
    };

    const el = (tag, className, content) => {
        const node = document.createElement(tag);
        if (className) node.className = className;
        if (content !== undefined) node.textContent = content;
        return node;
    };

    const render = () => {
        list.replaceChildren(...jobs.map(job => {
            const item = el('li', 'review-job');
            const main = el('div', 'review-job-main');
            const repo = el('span', 'review-job-repo', job.repo_dir.split('/').filter(Boolean).pop() || job.repo_dir);
            repo.title = job.repo_dir;
            main.append(repo, el('span', 'review-job-refs', `${job.from} \u2192 ${job.to}`));
            if (job.error) main.append(el('span', 'review-job-error', job.error));
            item.append(el('span', `review-job-status status-${job.status}`, text(job.status)), main, el('span', 'review-job-time', elapsed(job)));
            if (job.session_url) {
                const link = el('a', '', text('Open result'));
                link.href = job.session_url;
                item.append(link);
            }
            return item;
        }));
        empty.hidden = jobs.length > 0;
    };

    const applyLanguage = () => {
        document.documentElement.lang = language;
        panel.querySelectorAll('[data-en]').forEach(node => { node.textContent = text(node.dataset.en); });
        toggle.querySelectorAll('button').forEach(button => {
            button.setAttribute('aria-pressed', String(button.dataset.lang === language));
        });
        render();
    };

    let lastJobs = '';
    async function loadJobs() {
        try {
            const response = await fetch('/api/reviews');
            if (!response.ok) throw new Error(response.statusText);
            const body = await response.text();
            // Rebuilding the list on every poll would drop keyboard focus from its links.
            if (body === lastJobs) return;
            lastJobs = body;
            jobs = JSON.parse(body);
            render();
        } catch (_) {
            say('Could not load jobs', '', true);
        }
    }

    toggle.addEventListener('click', event => {
        const button = event.target.closest('button[data-lang]');
        if (!button || button.dataset.lang === language) return;
        language = button.dataset.lang;
        localStorage.setItem('ocr-viewer-language', language);
        applyLanguage();
    });

    // Source folder and base branch survive navigation; the review branch changes per review.
    const remembered = ['review-repo', 'review-from'];
    remembered.forEach(id => {
        const value = localStorage.getItem(`ocr-viewer-${id}`);
        if (value) document.getElementById(id).value = value;
    });
    const remember = () => remembered.forEach(id => localStorage.setItem(`ocr-viewer-${id}`, document.getElementById(id).value));
    form.addEventListener('input', remember);

    const post = (url, body) => fetch(url, {
        method: 'POST',
        headers: {'Content-Type': 'application/json', 'X-Viewer-Token': panel.dataset.token},
        body: body === undefined ? undefined : JSON.stringify(body)
    });

    const busy = async (button, work) => {
        button.disabled = true;
        try {
            await work();
        } finally {
            button.disabled = false;
        }
    };

    const pickFolder = document.getElementById('review-pick-folder');
    pickFolder.addEventListener('click', () => busy(pickFolder, async () => {
        try {
            const response = await post('/api/pick-folder');
            if (!response.ok) throw new Error((await response.text()).trim());
            document.getElementById('review-repo').value = (await response.json()).path;
            remember();
            say('');
            document.getElementById('review-fetch-branches').click();
        } catch (error) {
            say('Could not select folder', error.message, true);
        }
    }));

    const fetchBranches = document.getElementById('review-fetch-branches');
    fetchBranches.addEventListener('click', () => busy(fetchBranches, async () => {
        try {
            const response = await post('/api/branches', {repo_dir: document.getElementById('review-repo').value});
            if (!response.ok) throw new Error((await response.text()).trim());
            const branches = await response.json();
            document.getElementById('origin-branches').replaceChildren(...branches.map(branch => {
                const option = document.createElement('option');
                option.value = branch;
                return option;
            }));
            const from = document.getElementById('review-from');
            if (branches.includes('origin/develop') && from.value === 'develop') from.value = 'origin/develop';
            remember();
            say('Branches loaded', String(branches.length));
        } catch (error) {
            say('Could not fetch branches', error.message, true);
        }
    }));

    form.addEventListener('submit', async event => {
        event.preventDefault();
        const button = form.querySelector('.review-submit');
        button.disabled = true;
        try {
            const response = await post('/api/reviews', Object.fromEntries(new FormData(form)));
            if (!response.ok) throw new Error((await response.text()).trim());
            say('Review queued');
            document.getElementById('review-to').value = '';
            await loadJobs();
        } catch (error) {
            say('Review could not start', error.message, true);
        } finally {
            button.disabled = false;
        }
    });

    // ~/.opencodereview/config.json, the same file `ocr config` writes. The key
    // never comes back from the server; an empty field keeps the stored one.
    const llmForm = document.getElementById('llm-config-form');
    const llmField = name => llmForm.elements[name];
    let llmProviders = [];
    const fillModels = () => {
        const provider = llmProviders.find(p => p.name === llmField('provider').value);
        document.getElementById('llm-models').replaceChildren(...(provider ? provider.models : []).map(model => {
            const option = document.createElement('option');
            option.value = model;
            return option;
        }));
        llmField('url').placeholder = provider && provider.base_url ? provider.base_url : 'https://...';
    };
    const showLLMConfig = config => {
        llmProviders = config.providers;
        // Keep the configured provider selectable even if it is not in the list.
        if (config.provider && !llmProviders.some(p => p.name === config.provider)) {
            llmProviders.push({name: config.provider, label: 'custom', base_url: '', models: []});
        }
        llmField('provider').replaceChildren(...llmProviders.map(p => new Option(`${p.name} - ${p.label}`, p.name)));
        llmField('provider').value = savedProvider = config.provider;
        llmField('url').value = config.url;
        llmField('model').value = config.model;
        llmField('api_key').value = '';
        llmField('api_key').placeholder = config.key_set ? text('Saved; leave empty to keep') : '';
        fillModels();
    };
    let savedProvider = '';
    llmField('provider').addEventListener('input', () => {
        // Another provider's endpoint and model would be saved under the new name.
        if (llmField('provider').value !== savedProvider) {
            llmField('url').value = '';
            llmField('model').value = '';
        }
        fillModels();
    });
    const loadLLMConfig = async () => {
        try {
            const response = await fetch('/api/llm-config', {headers: {'X-Viewer-Token': panel.dataset.token}});
            if (!response.ok) throw new Error((await response.text()).trim());
            showLLMConfig(await response.json());
        } catch (error) {
            say('Could not load settings', error.message, true);
        }
    };
    llmForm.addEventListener('submit', async event => {
        event.preventDefault();
        const button = llmForm.querySelector('.review-submit');
        await busy(button, async () => {
            try {
                const response = await post('/api/llm-config', Object.fromEntries(new FormData(llmForm)));
                if (!response.ok) throw new Error((await response.text()).trim());
                showLLMConfig(await response.json());
                say('Settings saved');
            } catch (error) {
                say('Could not save settings', error.message, true);
            }
        });
    });
    document.getElementById('llm-config').addEventListener('toggle', event => {
        if (event.target.open) loadLLMConfig();
    });

    applyLanguage();
    loadJobs();
    // ponytail: fixed 3s polling; switch to SSE if job counts grow large.
    setInterval(() => {
        if (!document.hidden) loadJobs();
    }, 3000);
    setInterval(() => {
        list.querySelectorAll('.review-job-time').forEach((node, i) => {
            if (jobs[i] && !jobs[i].finished) node.textContent = elapsed(jobs[i]);
        });
    }, 1000);
})();
