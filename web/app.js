(() => {
  const FETCH_EVERY = 15000;
  const DAY = 86400000;
  const WINDOW_DAYS = 56;
  const RULE_COLOR = { merge: 'green', fix: 'red', review: 'blue' };
  const CAT_CLASS = { new: 'todo', indeterminate: 'prog', done: 'done' };

  const $ = (id) => document.getElementById(id);
  let state = null;

  function el(tag, attrs, ...children) {
    const n = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs || {})) {
      if (v === false || v == null) continue;
      if (k === 'class') n.className = v;
      else if (k === 'text') n.textContent = v;
      else if (k === 'hidden') n.hidden = v;
      else if (k.startsWith('on')) n.addEventListener(k.slice(2), v);
      else n.setAttribute(k, v);
    }
    for (const c of children.flat()) {
      if (c == null || c === false) continue;
      n.append(c.nodeType ? c : document.createTextNode(String(c)));
    }
    return n;
  }

  function ago(iso) {
    if (!iso) return '';
    const s = Math.max(0, (Date.now() - Date.parse(iso)) / 1000);
    if (s < 60) return `${Math.floor(s)}s`;
    if (s < 3600) return `${Math.floor(s / 60)}m`;
    if (s < 86400) return `${Math.floor(s / 3600)}h`;
    return `${Math.floor(s / 86400)}d`;
  }

  function parseInterval(s) {
    const m = /^(?:(\d+)h)?(?:(\d+)m)?(?:([\d.]+)s)?$/.exec(s || '');
    if (!m) return 60000;
    return ((+m[1] || 0) * 3600 + (+m[2] || 0) * 60 + (+m[3] || 0)) * 1000 || 60000;
  }

  function laneId(repo) { return 'lane-' + repo.ghRepo; }

  async function ack(ids) {
    ids = ids.filter(Boolean);
    if (!ids.length) return;
    try {
      await fetch('/api/ack', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ ids }) });
    } catch (e) { /* next refresh shows the truth */ }
    await refresh();
  }

  function ackButton(id) {
    return el('button', { class: 'ack', title: 'mark seen', type: 'button', text: 'x', onclick: () => ack([id]) });
  }

  function markAll(label, ids) {
    const unseen = ids.filter(Boolean);
    if (!unseen.length) return null;
    return el('div', { class: 'foot' }, el('button', { type: 'button', text: label, onclick: () => ack(unseen) }));
  }

  function dot(seen) { return el('span', { class: 'dot' + (seen ? ' seen' : '') }); }

  function unseenCount() {
    const lists = [state.items, state.mentions, state.assignedNew, state.selectedNew];
    return lists.reduce((n, l) => n + l.filter((x) => !x.seen).length, 0);
  }

  function itemsByRepo(items) {
    const map = new Map();
    for (const it of items) {
      if (!map.has(it.repo)) map.set(it.repo, []);
      map.get(it.repo).push(it);
    }
    return map;
  }

  function sortRepos(repos, byRepo) {
    const tickets = (r) => (r.next && r.next.tickets ? r.next.tickets.length : 0);
    return [...repos].sort((a, b) =>
      ((byRepo.get(b.ghRepo) || []).length - (byRepo.get(a.ghRepo) || []).length) ||
      (b.prs.length - a.prs.length) ||
      (tickets(b) - tickets(a)) ||
      a.name.localeCompare(b.name));
  }

  function renderPRs(repos, byRepo) {
    const root = $('prs');
    root.replaceChildren();
    const needMe = state.items.filter((i) => !i.seen).length;
    root.append(el('h2', {}, 'PRs ', el('span', { class: 'n', text: needMe ? `${needMe} need me` : '' })));
    const quiet = [];
    for (const r of repos) {
      const mine = byRepo.get(r.ghRepo) || [];
      if (!r.prs.length) { quiet.push(r); continue; }
      const counts = {};
      for (const it of mine) counts[it.rule] = (counts[it.rule] || 0) + 1;
      const badges = Object.entries(RULE_COLOR)
        .filter(([rule]) => counts[rule])
        .map(([rule, color]) => el('span', { class: `badge ${color}`, text: counts[rule] }));
      if (!badges.length) badges.push(el('span', { class: 'badge', text: r.prs.length }));
      root.append(el('a', { class: 'repo' + (mine.some((i) => i.rule !== 'review') ? ' hot' : ''), href: '#' + laneId(r) },
        el('div', { class: 'line1' }, el('b', { text: r.name }), el('span', { class: 'badges' }, badges)),
        mine.length ? el('div', { class: 'items' }, mine.map((it) => el('div', { class: it.seen ? 'seenrow' : '' },
          dot(it.seen),
          el('span', { class: `k ${RULE_COLOR[it.rule]}`, text: it.rule }),
          el('span', { class: 't', text: `#${it.number} ${it.title}` }),
          el('span', { class: 'sub', text: it.detail || ago(it.updatedAt) }),
          it.seen ? null : ackButton(it.id)))) : null));
    }
    for (const r of quiet) {
      root.append(el('div', { class: 'quiet' }, el('span', { text: r.name }), el('span', { text: 'no open PRs' })));
    }
    root.append(markAll('mark all seen', state.items.filter((i) => !i.seen).map((i) => i.id)));
  }

  function renderJira() {
    const root = $('jira');
    root.replaceChildren();
    const all = [...state.mentions, ...state.assignedNew, ...state.selectedNew];
    const unseen = all.filter((x) => !x.seen);
    root.append(el('h2', {}, 'Jira ', el('span', { class: 'n', text: unseen.length || '' })));
    const row = (cls, x, kind, meta, text) => el('div', { class: `row ${cls}` + (x.seen ? ' seenrow' : '') },
      dot(x.seen), el('span', { class: 'k', text: kind }), meta,
      x.seen ? null : ackButton(x.id),
      el('span', { class: 't' }, el('a', { href: x.url, target: '_blank', text: x.key }), ' ' + text));
    for (const m of state.mentions) {
      root.append(row(m.seen ? '' : 'ask', m, 'answer', el('span', { class: 'sub', text: `${m.lastCommentBy} · ${ago(m.updated)}` }), m.summary));
    }
    for (const a of state.assignedNew) {
      root.append(row('', a, 'assigned', el('span', { class: 'chip ' + statusColor(a.status), text: a.status }), `${a.type}: ${a.summary}`));
    }
    for (const s of state.selectedNew) {
      root.append(row('', s, 'selected', el('span', { class: 'sub', text: `${s.assignee || 'unassigned'} · ${ago(s.movedAt)}` }), s.summary));
    }
    if (!all.length) root.append(el('div', { class: 'row empty', text: 'nothing new' }));
    root.append(markAll('mark all seen', unseen.map((x) => x.id)));
  }

  function statusColor(status) {
    const s = (status || '').toLowerCase();
    if (s === 'done' || s === 'closed') return 'green';
    if (s === 'open' || s.startsWith('selected') || s === 'backlog') return 'grey';
    return 'blue';
  }

  function renderOps() {
    const root = $('ops');
    root.replaceChildren();
    const b = state.budget.github || {};
    const interval = parseInterval(b.interval);
    const polled = state.polledAt && !state.polledAt.startsWith('0001') ? Date.parse(state.polledAt) : 0;
    const stale = !polled || Date.now() - polled > 3 * interval;
    root.className = 'ops' + (stale ? ' stale' : '');
    root.append(el('span', {}, 'polled ', el('b', { text: polled ? ago(state.polledAt) : 'never' }), ' ago · interval ' + (b.interval || '-')));
    root.append(el('span', {}, 'GH ', el('b', { text: b.cost ?? '-' }), ' pts/poll · ', el('b', { text: b.remaining ?? '-' }), ' left'));
    const e = state.errors || {};
    root.append(el('span', {}, 'GitHub ', el('b', { text: e.github ? 'error' : 'ok' }), ' · Jira ', el('b', { text: e.jira ? 'error' : 'ok' })));
    if (e.github) root.append(el('span', { class: 'sub', text: e.github }));
    if (e.jira) root.append(el('span', { class: 'sub', text: e.jira }));
  }

  function dayOffset(dateStr) {
    if (!dateStr) return null;
    const d = Date.parse(dateStr + 'T12:00:00');
    const today = new Date(); today.setHours(12, 0, 0, 0);
    return Math.round((d - today.getTime()) / DAY);
  }

  function pct(days) { return 50 + (days / WINDOW_DAYS) * 100; }

  function renderRuler() {
    const root = $('ruler');
    root.replaceChildren();
    const fmt = (d) => d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
    for (let w = -4; w <= 4; w++) {
      const d = new Date(); d.setDate(d.getDate() + w * 7);
      root.append(el('span', { class: w === 0 ? 'today' : '', style: `left:${pct(w * 7)}%`, text: w === 0 ? 'today' : fmt(d) }));
    }
  }

  function marker(v, cls, label) {
    const days = dayOffset(v.releaseDate);
    if (days == null) return el('span', { class: `mk ${cls} undated` }, el('i'), label + ' · no date');
    if (days < -WINDOW_DAYS / 2) return el('span', { class: `mk ${cls} off` }, el('i'), `‹ ${label} · ${v.releaseDate}`);
    const rel = days === 0 ? 'today' : days > 0 ? `in ${days}d` : `${-days}d ago`;
    return el('span', { class: `mk ${cls}`, style: `left:${Math.min(pct(days), 100)}%` }, el('i'), `${label} · ${rel}`);
  }

  function renderLanes(repos, byRepo) {
    const root = $('lanes');
    root.replaceChildren();
    for (const r of repos) {
      const mine = new Set((byRepo.get(r.ghRepo) || []).map((i) => i.number));
      const quiet = !r.prs.length;
      const name = el('div', { class: 'name' }, el('h3', { text: r.name }));
      if (r.next && r.next.tickets) {
        const counts = { todo: 0, prog: 0, done: 0 };
        for (const t of r.next.tickets) counts[CAT_CLASS[t.category] || 'todo']++;
        const total = r.next.tickets.length || 1;
        name.append(el('div', { class: 'roll' }, ['done', 'prog', 'todo'].map((k) => el('i', { class: k, style: `width:${(counts[k] / total) * 100}%` }))));
        name.append(el('span', { class: 'rolltxt', text: `${r.next.name} · ${r.next.tickets.length} · ${counts.done} done · ${counts.prog} in prog · ${counts.todo} open` }));
      }
      const track = el('div', { class: 'track' }, el('span', { class: 'now' }));
      if (r.live) track.append(marker(r.live, '', r.live.name.replace(r.prefix, '').trim()));
      if (r.next) {
        const extra = r.queued.length ? ` · +${r.queued.length} queued` : '';
        const m = marker(r.next, 'next', r.next.name.replace(r.prefix, '').trim());
        m.append(extra);
        track.append(m);
        r.queued.forEach((q, i) => {
          const mk = el('span', { class: 'mk next small undated', style: `top:${28 + i * 14}px` }, el('i'), q.replace(r.prefix, '').trim());
          track.append(mk);
        });
      }
      if (!r.prefix) track.append(el('span', { class: 'mk off sub' }, 'no fixVersions'));
      const lane = el('div', { class: 'lane' + (quiet ? ' quiet' : ''), id: laneId(r) }, name, track);
      if (!quiet) {
        lane.append(el('div', { class: 'prs' }, r.prs.map((p) => el('div', { class: 'row' + (mine.has(p.number) ? ' me' : '') },
          el('a', { class: 'num', href: p.url, target: '_blank', text: '#' + p.number }),
          el('span', { class: 'grow' }, p.title, el('span', { class: 'sub', text: ` · ${p.author}` })),
          el('span', { class: 'rv', text: reviewers(p) }),
          reviewChip(p),
          el('span', { class: `ci ${p.checks}`, text: p.checks === 'ok' ? '✓ ci' : p.checks === 'fail' ? '✗ ci' : p.checks === 'pending' ? '… ci' : '–' })))));
      }
      root.append(lane);
    }
  }

  function reviewers(p) {
    const parts = (p.reviewers || []).map((r) => {
      const [who, st] = r.split(':');
      return `${who} ${st === 'APPROVED' ? '✓' : st === 'CHANGES_REQUESTED' ? '✗' : '·'}`;
    });
    if (p.requested && p.requested.length) parts.push('requested: ' + p.requested.join(', '));
    return parts.join(' · ');
  }

  function reviewChip(p) {
    if (p.draft) return el('span', { class: 'chip grey', text: 'draft' });
    if (p.mergeable === 'CONFLICTING') return el('span', { class: 'chip amber', text: 'conflict' });
    if (p.reviewDecision === 'CHANGES_REQUESTED') return el('span', { class: 'chip solid red', text: 'changes requested' });
    if (p.reviewDecision === 'APPROVED') return el('span', { class: 'chip solid green', text: 'approved' });
    if (p.requested && p.requested.length) return el('span', { class: 'chip', text: 'review' });
    return el('span', { class: 'chip grey', text: 'open' });
  }

  function render() {
    if (!state) return;
    const byRepo = itemsByRepo(state.items);
    const repos = sortRepos(state.repos, byRepo);
    const unseen = unseenCount();
    const count = $('brand-count');
    count.hidden = !unseen;
    count.textContent = unseen;
    document.title = unseen ? `(${unseen}) devboard` : 'devboard';
    renderPRs(repos, byRepo);
    renderJira();
    renderOps();
    renderRuler();
    renderLanes(repos, byRepo);
  }

  async function refresh() {
    try {
      const r = await fetch('/api/state', { cache: 'no-store' });
      if (r.ok) state = await r.json();
    } catch (e) {
      if (state) state.errors = { ...state.errors, github: 'server unreachable' };
    }
    render();
  }

  refresh();
  setInterval(refresh, FETCH_EVERY);
  setInterval(renderOps, 5000);
})();
