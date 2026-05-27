// ── Dashboard Application ──────────────────────────────────────
(function () {
    'use strict';

    const API_BASE = '/v1/admin/dashboard';
    let currentPage = 'overview';
    let paymentsOffset = 0;
    const paymentsLimit = 20;

    // ── Navigation ──────────────────────────────────────────────
    document.querySelectorAll('.nav-item').forEach(item => {
        item.addEventListener('click', (e) => {
            e.preventDefault();
            const page = item.dataset.page;
            switchPage(page);
        });
    });

    function switchPage(page) {
        currentPage = page;

        document.querySelectorAll('.nav-item').forEach(n => n.classList.remove('active'));
        document.querySelector(`[data-page="${page}"]`).classList.add('active');

        document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
        document.getElementById(`page-${page}`).classList.add('active');

        const titles = {
            'overview': 'Overview',
            'payments': 'Payments',
            'ledger': 'Ledger',
            'psp-health': 'PSP Health',
            'activity': 'Activity'
        };
        document.getElementById('pageTitle').textContent = titles[page] || page;

        loadPageData(page);
    }

    // ── Data Loading ────────────────────────────────────────────
    function loadPageData(page) {
        switch (page) {
            case 'overview': loadOverview(); break;
            case 'payments': loadPayments(); break;
            case 'ledger': loadLedger(); break;
            case 'psp-health': loadPSPHealth(); break;
            case 'activity': loadActivity(); break;
        }
    }

    async function apiFetch(path) {
        try {
            const res = await fetch(API_BASE + path);
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            setServerStatus(true);
            return await res.json();
        } catch (err) {
            console.error('API error:', err);
            setServerStatus(false);
            return null;
        }
    }

    function setServerStatus(connected) {
        const dot = document.querySelector('.status-dot');
        const text = document.querySelector('.status-text');
        if (connected) {
            dot.className = 'status-dot connected';
            text.textContent = 'Connected';
        } else {
            dot.className = 'status-dot error';
            text.textContent = 'Disconnected';
        }
    }

    // ── Overview Page ───────────────────────────────────────────
    async function loadOverview() {
        const stats = await apiFetch('/stats');
        if (stats) {
            setText('statTotalPayments', stats.total_payments.toLocaleString());
            setText('statTotalRevenue', formatCurrency(stats.total_revenue));
            setText('statSuccessRate', stats.success_rate.toFixed(1) + '%');
            setText('statAvgPayment', formatCurrency(stats.avg_payment_amount));
            setText('statFailureRate', `failure: ${stats.failure_rate.toFixed(1)}%`);
            setText('statTodayPayments', `today: ${stats.today_payments}`);
            setText('statTodayRevenue', `today: ${formatCurrency(stats.today_revenue)}`);

            renderStatusChart(stats.status_breakdown);
            renderPSPDistribution(stats.psp_breakdown);
        }

        const volume = await apiFetch('/volume');
        if (volume && volume.data) {
            renderVolumeChart(volume.data);
        }
    }

    let volumeChartInstance = null;
    function renderVolumeChart(data) {
        const ctx = document.getElementById('volumeChart');
        if (!ctx) return;

        if (volumeChartInstance) volumeChartInstance.destroy();

        volumeChartInstance = new Chart(ctx, {
            type: 'line',
            data: {
                labels: data.map(d => d.date.slice(5)),
                datasets: [{
                    label: 'Revenue',
                    data: data.map(d => d.revenue / 100),
                    borderColor: '#6366f1',
                    backgroundColor: 'rgba(99, 102, 241, 0.08)',
                    fill: true,
                    tension: 0.4,
                    pointRadius: 2,
                    pointHoverRadius: 5,
                    borderWidth: 2,
                }, {
                    label: 'Count',
                    data: data.map(d => d.count),
                    borderColor: '#3b82f6',
                    backgroundColor: 'transparent',
                    tension: 0.4,
                    pointRadius: 2,
                    borderWidth: 1.5,
                    borderDash: [4, 4],
                    yAxisID: 'y1',
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                interaction: { mode: 'index', intersect: false },
                plugins: {
                    legend: { labels: { color: '#94a3b8', font: { size: 11 } } }
                },
                scales: {
                    x: { ticks: { color: '#64748b', font: { size: 10 } }, grid: { color: 'rgba(148,163,184,0.05)' } },
                    y: { ticks: { color: '#64748b', font: { size: 10 } }, grid: { color: 'rgba(148,163,184,0.05)' } },
                    y1: { position: 'right', ticks: { color: '#64748b', font: { size: 10 } }, grid: { display: false } }
                }
            }
        });
    }

    let statusChartInstance = null;
    function renderStatusChart(breakdown) {
        const ctx = document.getElementById('statusChart');
        if (!ctx) return;
        if (statusChartInstance) statusChartInstance.destroy();

        const labels = Object.keys(breakdown);
        const values = Object.values(breakdown);
        const colors = labels.map(l => statusColor(l));

        statusChartInstance = new Chart(ctx, {
            type: 'doughnut',
            data: {
                labels: labels,
                datasets: [{ data: values, backgroundColor: colors, borderWidth: 0 }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                cutout: '65%',
                plugins: {
                    legend: { position: 'bottom', labels: { color: '#94a3b8', font: { size: 11 }, padding: 12 } }
                }
            }
        });
    }

    function renderPSPDistribution(breakdown) {
        const container = document.getElementById('pspDistribution');
        if (!container) return;
        const total = Object.values(breakdown).reduce((a, b) => a + b, 0) || 1;

        container.innerHTML = Object.entries(breakdown).map(([psp, count]) => {
            const pct = (count / total * 100).toFixed(0);
            return `
                <div class="psp-bar-row">
                    <span class="psp-bar-label">${psp}</span>
                    <div class="psp-bar-track"><div class="psp-bar-fill" style="width:${pct}%"></div></div>
                    <span class="psp-bar-count">${count}</span>
                </div>`;
        }).join('');
    }

    // ── Payments Page ───────────────────────────────────────────
    let allPayments = [];
    let filteredPayments = [];
    let searchQuery = '';
    let statusFilterValue = '';

    async function loadPayments() {
        const data = await apiFetch(`/payments?offset=${paymentsOffset}&limit=200`);
        if (!data) return;

        allPayments = data.payments || [];
        applyFilters();
    }

    function applyFilters() {
        filteredPayments = allPayments.filter(p => {
            // Status filter
            if (statusFilterValue && p.status !== statusFilterValue) return false;

            // Search filter
            if (searchQuery) {
                const q = searchQuery.toLowerCase();
                const searchable = [
                    p.id, p.order_id, p.merchant_id, p.psp,
                    p.customer_email || '', p.currency
                ].join(' ').toLowerCase();
                if (!searchable.includes(q)) return false;
            }

            return true;
        });

        renderPaymentsTable();
    }

    function renderPaymentsTable() {
        const start = paymentsOffset;
        const end = start + paymentsLimit;
        const page = filteredPayments.slice(start, end);

        const tbody = document.getElementById('paymentsBody');
        tbody.innerHTML = page.map(p => `
            <tr>
                <td class="id-cell">${p.id.slice(0, 8)}…</td>
                <td>${p.order_id}</td>
                <td>${formatCurrency(p.amount)}</td>
                <td>${p.currency}</td>
                <td><span class="status-badge ${p.status}">${p.status}</span></td>
                <td>${p.psp}</td>
                <td>${formatTime(p.created_at)}</td>
            </tr>
        `).join('') || '<tr><td colspan="7" style="text-align:center;color:#64748b;padding:40px">No payments match your filters</td></tr>';

        renderPagination(filteredPayments.length, paymentsOffset, paymentsLimit);
    }

    function renderPagination(total, offset, limit) {
        const container = document.getElementById('paymentsPagination');
        const totalPages = Math.ceil(total / limit);
        const currentPageNum = Math.floor(offset / limit) + 1;

        container.innerHTML = `
            <button ${offset === 0 ? 'disabled' : ''} onclick="window.__prevPage()">← Prev</button>
            <span class="page-info">${currentPageNum} / ${totalPages || 1} (${total})</span>
            <button ${offset + limit >= total ? 'disabled' : ''} onclick="window.__nextPage()">Next →</button>
        `;
    }

    window.__prevPage = () => { paymentsOffset = Math.max(0, paymentsOffset - paymentsLimit); renderPaymentsTable(); };
    window.__nextPage = () => { paymentsOffset += paymentsLimit; renderPaymentsTable(); };

    // Debounced search
    function debounce(fn, delay) {
        let timer;
        return function (...args) {
            clearTimeout(timer);
            timer = setTimeout(() => fn.apply(this, args), delay);
        };
    }

    document.getElementById('paymentSearch').addEventListener('input', debounce((e) => {
        searchQuery = e.target.value.trim();
        paymentsOffset = 0;
        applyFilters();
    }, 300));

    document.getElementById('statusFilter').addEventListener('change', (e) => {
        statusFilterValue = e.target.value;
        paymentsOffset = 0;
        applyFilters();
    });

    // ── Ledger Page ─────────────────────────────────────────────
    async function loadLedger() {
        // Load account balances
        const accounts = ['PSP_SETTLEMENT', 'MERCHANT_PAY', 'PLATFORM_FEE', 'REFUND_EXP'];
        const container = document.getElementById('ledgerAccounts');
        container.innerHTML = '';

        for (const code of accounts) {
            try {
                const res = await fetch(`/v1/ledger/accounts/${code}/balance`);
                const data = await res.json();
                if (data.success) {
                    container.innerHTML += `
                        <div class="ledger-card">
                            <div class="ledger-card-title">${code.replace(/_/g, ' ')}</div>
                            <div class="ledger-balance">${formatCurrency(data.data.current_balance)}</div>
                            <div class="ledger-type">${data.data.account_type}</div>
                        </div>`;
                }
            } catch (e) {
                container.innerHTML += `
                    <div class="ledger-card">
                        <div class="ledger-card-title">${code.replace(/_/g, ' ')}</div>
                        <div class="ledger-balance" style="color:var(--text-muted)">—</div>
                        <div class="ledger-type">unavailable</div>
                    </div>`;
            }
        }

        // Load recent journal entries
        try {
            const res = await fetch('/v1/ledger/entries?limit=50');
            const data = await res.json();
            if (data.success && data.data && data.data.entries) {
                renderLedgerEntries(data.data.entries);
            }
        } catch (e) {
            console.error('Failed to load ledger entries:', e);
        }
    }

    function renderLedgerEntries(entries) {
        const tbody = document.getElementById('ledgerEntriesBody');
        if (!tbody) return;

        tbody.innerHTML = (entries || []).map(e => {
            const desc = e.description || '—';
            const debitStr = e.debit > 0 ? formatCurrency(e.debit) : '';
            const creditStr = e.credit > 0 ? formatCurrency(e.credit) : '';
            const txBadgeClass = e.transaction_type === 'payment_capture' ? 'captured' :
                                 e.transaction_type === 'refund' ? 'refunded' : 'created';
            return `
                <tr>
                    <td>${formatTime(e.created_at)}</td>
                    <td><strong>${e.account_code}</strong><br><span style="color:var(--text-muted);font-size:0.72rem">${e.account_name}</span></td>
                    <td><span class="status-badge ${txBadgeClass}">${e.transaction_type}</span></td>
                    <td style="color:${e.debit > 0 ? 'var(--accent-green)' : 'var(--text-muted)'}">${debitStr}</td>
                    <td style="color:${e.credit > 0 ? 'var(--accent-red)' : 'var(--text-muted)'}">${creditStr}</td>
                    <td>${formatCurrency(e.running_balance)}</td>
                    <td style="color:var(--text-muted);font-size:0.8rem">${escapeHtml(desc)}</td>
                </tr>`;
        }).join('') || '<tr><td colspan="7" style="text-align:center;color:#64748b;padding:40px">No ledger entries yet</td></tr>';
    }

    // ── PSP Health Page ─────────────────────────────────────────
    async function loadPSPHealth() {
        const data = await apiFetch('/psp-health');
        if (!data || !data.providers) return;

        const container = document.getElementById('pspHealthCards');
        container.innerHTML = data.providers.map(p => `
            <div class="psp-card">
                <div class="psp-card-header">
                    <span class="psp-name">${p.psp.toUpperCase()}</span>
                    <span class="circuit-badge ${p.circuit_state}">${p.circuit_state}</span>
                </div>
                <div class="psp-metrics">
                    <div class="psp-metric">
                        <div class="psp-metric-label">Total Requests</div>
                        <div class="psp-metric-value">${p.total_requests}</div>
                    </div>
                    <div class="psp-metric">
                        <div class="psp-metric-label">Failures</div>
                        <div class="psp-metric-value" style="color:${p.total_failures > 0 ? 'var(--accent-red)' : 'inherit'}">${p.total_failures}</div>
                    </div>
                    <div class="psp-metric">
                        <div class="psp-metric-label">Success Rate</div>
                        <div class="psp-metric-value">${p.success_rate.toFixed(1)}%</div>
                    </div>
                    <div class="psp-metric">
                        <div class="psp-metric-label">DB Captured</div>
                        <div class="psp-metric-value">${p.db_captured_count}</div>
                    </div>
                </div>
            </div>
        `).join('');
    }

    // ── Activity Page ───────────────────────────────────────────
    async function loadActivity() {
        const data = await apiFetch('/activity?limit=50');
        if (!data || !data.events) return;

        const container = document.getElementById('activityFeed');
        container.innerHTML = (data.events || []).map(e => `
            <div class="activity-item">
                <div class="activity-icon ${e.event_type}">${e.event_type === 'webhook' ? 'WH' : 'ST'}</div>
                <div class="activity-body">
                    <div class="activity-desc">${escapeHtml(e.description)}</div>
                    <div class="activity-meta">
                        ${e.payment_id ? `Payment: ${e.payment_id.slice(0, 8)}… · ` : ''}
                        ${e.actor} · ${formatTime(e.timestamp)}
                    </div>
                </div>
            </div>
        `).join('') || '<div style="color:var(--text-muted);padding:40px;text-align:center">No activity yet</div>';
    }

    // ── Utilities ───────────────────────────────────────────────
    function setText(id, text) {
        const el = document.getElementById(id);
        if (el) el.textContent = text;
    }

    function formatCurrency(amount) {
        if (typeof amount !== 'number') return '—';
        return '₹' + (amount / 100).toLocaleString('en-IN', { minimumFractionDigits: 2 });
    }

    function formatTime(ts) {
        if (!ts) return '—';
        const d = new Date(ts);
        return d.toLocaleDateString('en-IN', { day: '2-digit', month: 'short' }) + ' ' +
               d.toLocaleTimeString('en-IN', { hour: '2-digit', minute: '2-digit' });
    }

    function statusColor(status) {
        const map = {
            captured: '#10b981', authorized: '#3b82f6', created: '#94a3b8',
            failed: '#ef4444', refunded: '#f59e0b', cancelled: '#64748b',
            partially_refunded: '#f59e0b'
        };
        return map[status] || '#64748b';
    }

    function escapeHtml(str) {
        const div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    }

    // ── Refresh Button ──────────────────────────────────────────
    document.getElementById('refreshBtn').addEventListener('click', () => {
        const btn = document.getElementById('refreshBtn');
        btn.classList.add('spinning');
        loadPageData(currentPage);
        setTimeout(() => btn.classList.remove('spinning'), 800);
    });

    // ── Mobile Menu Toggle ──────────────────────────────────────
    document.getElementById('menuToggle').addEventListener('click', () => {
        document.getElementById('sidebar').classList.toggle('open');
    });

    // ── Initial Load ────────────────────────────────────────────
    loadOverview();

})();
