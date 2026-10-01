// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

(() => {
    const box = document.getElementById('share-actions');
    if (!box) return;
    const status = box.querySelector('[data-share-status]');
    const esc = s => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

    // Reads the visible cards so the copy matches the screen, including the
    // finding language currently selected and any Fixed/Ignored marks.
    const findings = () => [...document.querySelectorAll('[data-comment-card]')]
        .filter(card => !card.hidden && !card.closest('.comment-file-group')?.hidden)
        .map(card => ({
        severity: (card.dataset.severity || '').toUpperCase(),
        file: card.closest('.comment-file-group')?.querySelector('.file-path')?.textContent.trim() || '',
        line: card.querySelector('.comment-lines')?.textContent.trim() || '',
        text: card.querySelector('.comment-content')?.textContent.trim() || '',
        mark: card.querySelector('[data-mark-chip]:not([hidden])')?.textContent.trim() || ''
    }));

    box.querySelector('[data-share-copy]').addEventListener('click', async () => {
        const d = box.dataset;
        const range = d.from && d.to ? `${d.from} \u2192 ${d.to}` : d.branch;
        const items = findings();
        const where = f => f.file + (f.line ? ` ${f.line}` : '');
        const plain = [`Code review: ${d.repoName} (${range})`, `${items.length} findings`, '']
            .concat(items.map((f, i) => `${i + 1}. [${f.severity}] ${where(f)}${f.mark ? ` (${f.mark})` : ''}\n   ${f.text}`))
            .join('\n');
        const html = `<p><b>Code review: ${esc(d.repoName)}</b> <code>${esc(range)}</code><br>${items.length} findings</p><ol>`
            + items.map(f => `<li><b>[${esc(f.severity)}]</b> <code>${esc(where(f))}</code>${f.mark ? ` <i>(${esc(f.mark)})</i>` : ''}<br>${esc(f.text)}</li>`).join('')
            + '</ol>';
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
})();
