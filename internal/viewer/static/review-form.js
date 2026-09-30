// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

(() => {
    const panel = document.querySelector('.review-launcher');
    if (!panel) return;
    const form = document.getElementById('review-form');
    const message = document.getElementById('review-message');
    const list = document.getElementById('review-jobs');
    const toggle = document.getElementById('review-language');
    const pickFolder = document.getElementById('review-pick-folder');
    const fetchBranches = document.getElementById('review-fetch-branches');
    const translations = {
        'Start a review': 'B\u1eaft \u0111\u1ea7u review',
        'Source folder': 'Th\u01b0 m\u1ee5c source',
        'Base branch': 'Nh\u00e1nh \u0111\u1ed1i chi\u1ebfu',
        'Review branch': 'Nh\u00e1nh c\u1ea7n review',
        'Review jobs': 'C\u00e1c l\u01b0\u1ee3t review',
        'Review': 'Review',
        queued: '\u0110ang ch\u1edd', running: '\u0110ang review',
        completed: 'Ho\u00e0n t\u1ea5t', failed: 'Th\u1ea5t b\u1ea1i',
        'Open result': 'Xem k\u1ebft qu\u1ea3',
        'Could not load jobs': 'Kh\u00f4ng t\u1ea3i \u0111\u01b0\u1ee3c danh s\u00e1ch',
        'Review could not start': 'Kh\u00f4ng th\u1ec3 b\u1eaft \u0111\u1ea7u review',
        'Review queued': '\u0110\u00e3 x\u1ebfp h\u00e0ng review',
        'Choose folder': 'Ch\u1ecdn th\u01b0 m\u1ee5c',
        'Could not select folder': 'Kh\u00f4ng th\u1ec3 ch\u1ecdn th\u01b0 m\u1ee5c',
        'Fetch origin': 'T\u1ea3i nh\u00e1nh origin',
        'Could not fetch branches': 'Kh\u00f4ng t\u1ea3i \u0111\u01b0\u1ee3c nh\u00e1nh',
        'Branches loaded': '\u0110\u00e3 t\u1ea3i danh s\u00e1ch nh\u00e1nh'
    };
    let language = localStorage.getItem('ocr-viewer-language') === 'vi' ? 'vi' : 'en';
    const text = key => language === 'vi' ? (translations[key] || key) : key;
    const applyLanguage = () => {
        panel.querySelectorAll('[data-en]').forEach(el => { el.textContent = text(el.dataset.en); });
        toggle.setAttribute('aria-label', language === 'vi' ? 'Switch to English' : 'Switch to Vietnamese');
        toggle.setAttribute('aria-pressed', String(language === 'vi'));
        loadJobs();
    };
    toggle.addEventListener('click', () => {
        language = language === 'en' ? 'vi' : 'en';
        localStorage.setItem('ocr-viewer-language', language);
        applyLanguage();
    });
    async function loadJobs() {
        try {
            const response = await fetch('/api/reviews');
            if (!response.ok) throw new Error();
            const jobs = await response.json();
            list.replaceChildren();
            for (const job of jobs) {
                const item = document.createElement('li');
                item.textContent = `${job.repo_dir}: ${job.from} / ${job.to} - ${text(job.status)}`;
                if (job.error) item.append(document.createTextNode(`: ${job.error}`));
                if (job.session_url) {
                    const link = document.createElement('a');
                    link.href = job.session_url;
                    link.textContent = text('Open result');
                    item.append(' ', link);
                }
                list.append(item);
            }
        } catch (_) {
            message.textContent = text('Could not load jobs');
        }
    }
    pickFolder.addEventListener('click', async () => {
        pickFolder.disabled = true;
        try {
            const response = await fetch('/api/pick-folder', {
                method: 'POST', headers: {'X-Viewer-Token': panel.dataset.token}
            });
            if (!response.ok) throw new Error(await response.text());
            document.getElementById('review-repo').value = (await response.json()).path;
        } catch (error) {
            message.textContent = `${text('Could not select folder')}: ${error.message}`;
        } finally {
            pickFolder.disabled = false;
        }
    });
    fetchBranches.addEventListener('click', async () => {
        fetchBranches.disabled = true;
        try {
            const response = await fetch('/api/branches', {
                method: 'POST',
                headers: {'Content-Type': 'application/json', 'X-Viewer-Token': panel.dataset.token},
                body: JSON.stringify({repo_dir: document.getElementById('review-repo').value})
            });
            if (!response.ok) throw new Error(await response.text());
            const branches = await response.json();
            const options = document.getElementById('origin-branches');
            options.replaceChildren(...branches.map(branch => {
                const option = document.createElement('option');
                option.value = branch;
                return option;
            }));
            if (branches.includes('origin/develop') && document.getElementById('review-from').value === 'develop') {
                document.getElementById('review-from').value = 'origin/develop';
            }
            message.textContent = `${text('Branches loaded')}: ${branches.length}`;
        } catch (error) {
            message.textContent = `${text('Could not fetch branches')}: ${error.message}`;
        } finally {
            fetchBranches.disabled = false;
        }
    });
    form.addEventListener('submit', async event => {
        event.preventDefault();
        const button = form.querySelector('button[type="submit"]');
        button.disabled = true;
        try {
            const response = await fetch('/api/reviews', {
                method: 'POST',
                headers: {'Content-Type': 'application/json', 'X-Viewer-Token': panel.dataset.token},
                body: JSON.stringify(Object.fromEntries(new FormData(form)))
            });
            if (!response.ok) throw new Error(await response.text());
            message.textContent = text('Review queued');
            await loadJobs();
        } catch (error) {
            message.textContent = `${text('Review could not start')}: ${error.message}`;
        } finally {
            button.disabled = false;
        }
    });
    applyLanguage();
    setInterval(loadJobs, 3000);
})();
