// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

(() => {
    const controls = document.getElementById('finding-language');
    if (!controls) return;
    const status = document.getElementById('finding-translation-status');
    const findings = [...document.querySelectorAll('.comment-content[data-finding-id]')];
    const translated = new Map();
    let language = 'vi';
    const render = () => {
        controls.querySelectorAll('button').forEach(button => {
            button.setAttribute('aria-pressed', String(button.dataset.findingLanguage === language));
        });
        findings.forEach(finding => {
            finding.textContent = language === 'vi' ? (translated.get(finding.dataset.findingId) || finding.dataset.original) : finding.dataset.original;
        });
    };
    controls.addEventListener('click', event => {
        const next = event.target.closest('button[data-finding-language]');
        if (!next) return;
        language = next.dataset.findingLanguage;
        render();
    });
    render();
    if (findings.length === 0) return;
    status.textContent = 'Translating findings...';
    Promise.all(findings.map(async finding => {
        try {
            const response = await fetch('/api/translate', {
                method: 'POST',
                headers: {'Content-Type': 'application/json', 'X-Viewer-Token': controls.dataset.token},
                body: JSON.stringify({repo: controls.dataset.repo, session: controls.dataset.session, finding: finding.dataset.findingId})
            });
            if (!response.ok) throw new Error(await response.text());
            translated.set(finding.dataset.findingId, (await response.json()).translation);
            render();
            return true;
        } catch (_) {
            return false;
        }
    })).then(results => {
        status.textContent = results.every(Boolean) ? '' : 'Some translations are unavailable; showing original English.';
    });
})();
