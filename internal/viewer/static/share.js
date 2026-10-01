// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

(() => {
    const box = document.getElementById('share-actions');
    if (!box) return;
    const status = box.querySelector('[data-share-status]');
    const esc = s => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

    // Reads the visible cards so the list matches the screen, including the
    // finding language currently selected. Cards marked fixed or ignored are
    // left out: members only get what still needs changing.
    const findings = () => [...document.querySelectorAll('[data-comment-card]')]
        .filter(card => !card.hidden && !card.dataset.mark && !card.closest('.comment-file-group')?.hidden)
        .map(card => ({
            severity: (card.dataset.severity || '').toUpperCase(),
            file: card.closest('.comment-file-group')?.querySelector('.file-path')?.textContent.trim() || '',
            line: card.querySelector('.comment-lines')?.textContent.trim() || '',
            text: card.querySelector('.comment-content')?.textContent.trim() || ''
        }));

    const report = () => {
        const d = box.dataset;
        const range = d.from && d.to ? `${d.from} \u2192 ${d.to}` : d.branch;
        const items = findings();
        const where = f => f.file + (f.line ? ` ${f.line}` : '');
        const title = `Code review: ${d.repoName} (${range})`;
        const plain = [title, `${items.length} findings`, '']
            .concat(items.map((f, i) => `${i + 1}. [${f.severity}] ${where(f)}\n   ${f.text}`))
            .join('\n');
        const html = `<p><b>Code review: ${esc(d.repoName)}</b> <code>${esc(range)}</code><br>${items.length} findings</p><ol>`
            + items.map(f => `<li><b>[${esc(f.severity)}]</b> <code>${esc(where(f))}</code><br>${esc(f.text)}</li>`).join('')
            + '</ol>';
        return {title, plain, html};
    };

    box.querySelector('[data-share-copy]').addEventListener('click', async () => {
        const {plain, html} = report();
        try {
            if (window.ClipboardItem) {
                await navigator.clipboard.write([new ClipboardItem({
                    'text/html': new Blob([html], {type: 'text/html'}),
                    'text/plain': new Blob([plain], {type: 'text/plain'})
                })]);
            } else {
                await navigator.clipboard.writeText(plain);
            }
            status.textContent = 'Copied. Paste into Teams.';
        } catch (_) {
            status.textContent = 'Copy failed';
        }
    });

    box.querySelector('[data-share-download]').addEventListener('click', () => {
        const {title, html} = report();
        const page = `<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8"><title>${esc(title)}</title>`
            + '<style>body{font:15px/1.5 system-ui,sans-serif;max-width:860px;margin:2rem auto;padding:0 1rem;color:#1f2328}'
            + 'li{margin-bottom:1rem}code{background:#f0f2f5;padding:0 .3em;border-radius:4px}</style></head><body>'
            + html + '</body></html>';
        const link = document.createElement('a');
        link.href = URL.createObjectURL(new Blob([page], {type: 'text/html'}));
        link.download = `review-${box.dataset.repoName}-${box.dataset.session.slice(0, 8)}.html`;
        link.click();
        setTimeout(() => URL.revokeObjectURL(link.href), 1000);
    });
})();
