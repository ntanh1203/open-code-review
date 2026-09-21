// SPDX-License-Identifier: Apache-2.0

let currentCommentID = null;

// --- Intentional Modal ---

function openIntentionalModal(commentID) {
  currentCommentID = commentID;
  document.getElementById('modal-overlay').style.display = 'flex';
  document.getElementById('modal-reason').value = '';
  document.getElementById('modal-actor').value = localStorage.getItem('ocr-actor') || '';
  document.getElementById('modal-error').style.display = 'none';
  document.getElementById('modal-reason').focus();
}

function closeModal() {
  document.getElementById('modal-overlay').style.display = 'none';
  currentCommentID = null;
}

async function submitIntentional() {
  const reason = document.getElementById('modal-reason').value.trim();
  const actor = document.getElementById('modal-actor').value.trim();
  const errEl = document.getElementById('modal-error');

  if (!actor) {
    errEl.textContent = 'Please enter your name.';
    errEl.style.display = 'block';
    return;
  }
  if (reason.length < 10) {
    errEl.textContent = 'Reason must be at least 10 characters.';
    errEl.style.display = 'block';
    return;
  }

  localStorage.setItem('ocr-actor', actor);

  const res = await fetch(`/api/comment/${currentCommentID}/intentional`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ actor, reason })
  });

  if (!res.ok) {
    const data = await res.json();
    errEl.textContent = data.error || 'Failed';
    errEl.style.display = 'block';
    return;
  }

  closeModal();
  location.reload();
}

// --- Mark Fixed ---

async function markFixed(commentID) {
  let actor = localStorage.getItem('ocr-actor');
  if (!actor) {
    actor = prompt('Enter your name:');
    if (!actor) return;
    localStorage.setItem('ocr-actor', actor);
  }

  const res = await fetch(`/api/comment/${commentID}/fixed`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ actor })
  });

  if (res.ok) {
    location.reload();
  } else {
    const data = await res.json();
    alert(data.error || 'Failed');
  }
}

// --- Re-Review ---

async function reReview(reviewID) {
  if (!confirm('Run a new review for this MR? Intentional comments will be preserved.')) return;

  const res = await fetch(`/api/review/${reviewID}/re-review`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' }
  });

  if (res.ok) {
    const data = await res.json();
    alert(`Re-review queued (new review #${data.review_id})`);
    location.reload();
  } else {
    const data = await res.json();
    alert(data.error || 'Failed');
  }
}

// --- Add Repo ---

async function addRepo(e) {
  e.preventDefault();
  const form = e.target;
  const data = {
    gitlab_url: form.gitlab_url.value,
    project_id: parseInt(form.project_id.value),
    project_name: form.project_name.value,
    clone_path: form.clone_path.value,
    default_branch: form.default_branch.value || 'develop',
    api_token: form.api_token.value
  };

  const res = await fetch('/api/repo', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data)
  });

  if (res.ok) {
    location.reload();
  } else {
    const err = await res.json();
    alert(err.error || 'Failed');
  }
  return false;
}

// --- Trigger Review ---

function showTriggerForm() {
  document.getElementById('trigger-form').style.display = 'block';
}

function hideTriggerForm() {
  document.getElementById('trigger-form').style.display = 'none';
}

async function triggerReview(e, repoID) {
  e.preventDefault();
  const form = e.target;
  const data = {
    source_branch: form.source_branch.value,
    target_branch: form.target_branch.value,
    mr_iid: parseInt(form.mr_iid.value) || 0,
    author: form.author.value
  };

  const res = await fetch(`/api/repo/${repoID}/review`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data)
  });

  if (res.ok) {
    const result = await res.json();
    alert(`Review #${result.review_id} queued!`);
    location.reload();
  } else {
    const err = await res.json();
    alert(err.error || 'Failed');
  }
  return false;
}

// Close modal on escape
document.addEventListener('keydown', function(e) {
  if (e.key === 'Escape') closeModal();
});

// Close modal on overlay click
document.getElementById('modal-overlay')?.addEventListener('click', function(e) {
  if (e.target === this) closeModal();
});
