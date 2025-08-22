// Global state
let currentPage = 1;
const pageSize = 20;
let currentStatus = 'all';
let currentTab = 'metrics';
let currentProvider = 'gmail';

// Dashboard initialization
document.addEventListener('DOMContentLoaded', function() {
    initializeTabNavigation();
    initializeProviderTabs();
    initializeEventHandlers();
    loadInitialData();
    
    // Auto refresh data every 10 seconds
    setInterval(refreshCurrentTabData, 10000);
});

// Initialize main tab navigation
function initializeTabNavigation() {
    const mainTabs = document.querySelectorAll('.main-tab');
    
    mainTabs.forEach(tab => {
        tab.addEventListener('click', function() {
            const tabId = this.getAttribute('data-tab');
            switchMainTab(tabId);
        });
    });
}

// Initialize provider sub-tabs
function initializeProviderTabs() {
    const providerTabs = document.querySelectorAll('.provider-tab-btn');
    
    providerTabs.forEach(tab => {
        tab.addEventListener('click', function() {
            const provider = this.getAttribute('data-provider');
            switchProviderTab(provider);
        });
    });
}

// Initialize event handlers
function initializeEventHandlers() {
    // Refresh button
    const refreshBtn = document.getElementById('refresh-btn');
    if (refreshBtn) {
        refreshBtn.addEventListener('click', refreshCurrentTabData);
    }
    
    // Messages filters and pagination
    const statusFilter = document.getElementById('status-filter');
    if (statusFilter) {
        statusFilter.addEventListener('change', function(e) {
            currentStatus = e.target.value;
            currentPage = 1;
            loadMessages();
        });
    }
    
    const prevBtn = document.getElementById('prev-btn');
    if (prevBtn) {
        prevBtn.addEventListener('click', function() {
            if (currentPage > 1) {
                currentPage--;
                loadMessages();
            }
        });
    }
    
    const nextBtn = document.getElementById('next-btn');
    if (nextBtn) {
        nextBtn.addEventListener('click', function() {
            currentPage++;
            loadMessages();
        });
    }
    
    // Modal handlers
    const closeBtn = document.querySelector('.close');
    if (closeBtn) {
        closeBtn.addEventListener('click', closeModal);
    }
    
    window.addEventListener('click', function(e) {
        const modal = document.getElementById('message-modal');
        if (modal && e.target === modal) {
            closeModal();
        }
    });
}

// Load initial data based on active tab
function loadInitialData() {
    loadStats(); // Always load stats for metrics tab
    if (currentTab === 'messages') {
        loadMessages();
    }
    loadRateLimit(); // Always load for metrics and providers
}

// Switch main tabs
function switchMainTab(tabId) {
    // Update tab buttons
    document.querySelectorAll('.main-tab').forEach(tab => {
        tab.classList.remove('active');
    });
    document.querySelector(`[data-tab="${tabId}"]`).classList.add('active');
    
    // Update tab content
    document.querySelectorAll('.tab-content').forEach(content => {
        content.classList.remove('active');
    });
    document.getElementById(`${tabId}-content`).classList.add('active');
    
    // Update current tab and load appropriate data
    currentTab = tabId;
    loadTabData(tabId);
}

// Load data for specific tab
function loadTabData(tabId) {
    switch(tabId) {
        case 'metrics':
            loadStats();
            loadRateLimit();
            break;
        case 'providers':
            loadRateLimit();
            break;
        case 'pools':
            loadLoadBalancingData();
            break;
        case 'messages':
            loadMessages();
            break;
    }
}

// Refresh data for current active tab
function refreshCurrentTabData() {
    loadTabData(currentTab);
}

// Switch provider sub-tabs
function switchProviderTab(provider) {
    // Update provider tab buttons
    document.querySelectorAll('.provider-tab-btn').forEach(btn => {
        btn.classList.remove('active');
    });
    document.querySelector(`[data-provider="${provider}"]`).classList.add('active');
    
    // Update provider panels
    document.querySelectorAll('.provider-panel').forEach(panel => {
        panel.classList.remove('active');
    });
    document.getElementById(`${provider}-provider`).classList.add('active');
    
    currentProvider = provider;
    
    // Load data for specific provider if needed
    if (provider === 'loadbalancing') {
        loadLoadBalancingData();
    }
}

function loadStats() {
    fetch('/api/stats')
        .then(response => response.json())
        .then(data => {
            document.getElementById('total-count').textContent = data.total || 0;
            
            const statusCounts = data.statusCounts || [];
            let queued = 0, processing = 0, sent = 0, failed = 0;
            
            statusCounts.forEach(item => {
                switch(item.Status) {
                    case 'queued': queued = item.Count; break;
                    case 'processing': processing = item.Count; break;
                    case 'sent': sent = item.Count; break;
                    case 'failed': failed = item.Count; break;
                }
            });
            
            document.getElementById('queued-count').textContent = queued;
            document.getElementById('processing-count').textContent = processing;
            document.getElementById('sent-count').textContent = sent;
            document.getElementById('failed-count').textContent = failed;
        })
        .catch(error => console.error('Error loading stats:', error));
}

function loadMessages() {
    const offset = (currentPage - 1) * pageSize;
    let url = `/api/messages?limit=${pageSize}&offset=${offset}`;
    
    if (currentStatus !== 'all') {
        url += `&status=${currentStatus}`;
    }
    
    fetch(url)
        .then(response => response.json())
        .then(messages => {
            const tbody = document.getElementById('messages-tbody');
            tbody.innerHTML = '';
            
            if (messages.length === 0) {
                tbody.innerHTML = '<tr><td colspan="7" class="loading">No messages found</td></tr>';
                document.getElementById('next-btn').disabled = true;
            } else {
                messages.forEach(msg => {
                    const row = createMessageRow(msg);
                    tbody.appendChild(row);
                });
                
                document.getElementById('next-btn').disabled = messages.length < pageSize;
            }
            
            document.getElementById('prev-btn').disabled = currentPage === 1;
            document.getElementById('page-info').textContent = `Page ${currentPage}`;
        })
        .catch(error => {
            console.error('Error loading messages:', error);
            document.getElementById('messages-tbody').innerHTML = 
                '<tr><td colspan="7" class="loading">Error loading messages</td></tr>';
        });
}

function createMessageRow(message) {
    const row = document.createElement('tr');
    
    const idCell = document.createElement('td');
    idCell.textContent = message.id.substring(0, 8) + '...';
    idCell.style.fontFamily = 'monospace';
    idCell.style.fontSize = '12px';
    
    const fromCell = document.createElement('td');
    fromCell.textContent = message.from;
    
    const toCell = document.createElement('td');
    toCell.textContent = message.to ? message.to.join(', ') : '';
    
    const subjectCell = document.createElement('td');
    subjectCell.textContent = message.subject || '(no subject)';
    
    const statusCell = document.createElement('td');
    const statusSpan = document.createElement('span');
    statusSpan.className = `status ${message.status}`;
    statusSpan.textContent = message.status;
    
    if (message.status === 'auth_error') {
        statusSpan.title = 'Click to authorize Gmail';
        statusSpan.addEventListener('click', () => {
            const email = prompt('Enter the Gmail address to authorize:', '');
            if (email) {
                window.location.href = `/oauth/start?email=${encodeURIComponent(email)}`;
            }
        });
    }
    
    statusCell.appendChild(statusSpan);
    
    const queuedCell = document.createElement('td');
    queuedCell.textContent = formatDate(message.queued_at);
    
    const actionsCell = document.createElement('td');
    const viewBtn = document.createElement('button');
    viewBtn.textContent = 'View';
    viewBtn.style.marginRight = '5px';
    viewBtn.addEventListener('click', () => viewMessage(message.id));
    
    const deleteBtn = document.createElement('button');
    deleteBtn.textContent = 'Delete';
    deleteBtn.style.backgroundColor = '#e74c3c';
    deleteBtn.addEventListener('click', () => deleteMessage(message.id));
    
    actionsCell.appendChild(viewBtn);
    actionsCell.appendChild(deleteBtn);
    
    row.appendChild(idCell);
    row.appendChild(fromCell);
    row.appendChild(toCell);
    row.appendChild(subjectCell);
    row.appendChild(statusCell);
    row.appendChild(queuedCell);
    row.appendChild(actionsCell);
    
    return row;
}

function viewMessage(id) {
    fetch(`/api/messages/${id}`)
        .then(response => response.json())
        .then(message => {
            const details = document.getElementById('message-details');
            details.innerHTML = `
                <div class="detail-row">
                    <div class="detail-label">ID:</div>
                    <div class="detail-value">${message.id}</div>
                </div>
                <div class="detail-row">
                    <div class="detail-label">From:</div>
                    <div class="detail-value">${message.from}</div>
                </div>
                <div class="detail-row">
                    <div class="detail-label">To:</div>
                    <div class="detail-value">${message.to ? message.to.join(', ') : ''}</div>
                </div>
                ${message.cc && message.cc.length > 0 ? `
                <div class="detail-row">
                    <div class="detail-label">CC:</div>
                    <div class="detail-value">${message.cc.join(', ')}</div>
                </div>` : ''}
                ${message.bcc && message.bcc.length > 0 ? `
                <div class="detail-row">
                    <div class="detail-label">BCC:</div>
                    <div class="detail-value">${message.bcc.join(', ')}</div>
                </div>` : ''}
                <div class="detail-row">
                    <div class="detail-label">Subject:</div>
                    <div class="detail-value">${message.subject || '(no subject)'}</div>
                </div>
                <div class="detail-row">
                    <div class="detail-label">Status:</div>
                    <div class="detail-value"><span class="status ${message.status}">${message.status}</span></div>
                </div>
                <div class="detail-row">
                    <div class="detail-label">Queued At:</div>
                    <div class="detail-value">${formatDate(message.queued_at)}</div>
                </div>
                ${message.processed_at ? `
                <div class="detail-row">
                    <div class="detail-label">Processed At:</div>
                    <div class="detail-value">${formatDate(message.processed_at)}</div>
                </div>` : ''}
                ${message.error ? `
                <div class="error-message">
                    <strong>Error:</strong> ${message.error}
                </div>` : ''}
                ${message.html ? `
                <div class="detail-row">
                    <div class="detail-label">HTML Content:</div>
                    <div class="detail-value">
                        <div style="border: 1px solid #ddd; padding: 10px; max-height: 300px; overflow-y: auto;">
                            ${escapeHtml(message.html)}
                        </div>
                    </div>
                </div>` : ''}
                ${message.text ? `
                <div class="detail-row">
                    <div class="detail-label">Text Content:</div>
                    <div class="detail-value">
                        <pre style="border: 1px solid #ddd; padding: 10px; max-height: 300px; overflow-y: auto; white-space: pre-wrap;">${escapeHtml(message.text)}</pre>
                    </div>
                </div>` : ''}
            `;
            
            document.getElementById('message-modal').style.display = 'block';
        })
        .catch(error => {
            console.error('Error loading message:', error);
            alert('Error loading message details');
        });
}

function deleteMessage(id) {
    if (!confirm('Are you sure you want to delete this message?')) {
        return;
    }
    
    fetch(`/api/messages/${id}`, { method: 'DELETE' })
        .then(() => {
            loadStats();
            loadMessages();
        })
        .catch(error => {
            console.error('Error deleting message:', error);
            alert('Error deleting message');
        });
}

function closeModal() {
    document.getElementById('message-modal').style.display = 'none';
}

function formatDate(dateString) {
    const date = new Date(dateString);
    return date.toLocaleString();
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Legacy function - remove if not needed by existing HTML
function switchProviderTabLegacy(provider, event) {
    // Update button states
    document.querySelectorAll('.tab-button').forEach(btn => {
        btn.classList.remove('active');
    });
    
    // Find the button that was clicked and make it active
    if (event && event.target) {
        event.target.classList.add('active');
    } else {
        // Fallback: find button by provider name
        document.querySelectorAll('.tab-button').forEach(btn => {
            if (btn.textContent.toLowerCase().includes(provider.toLowerCase()) ||
                (provider === 'all' && btn.textContent.includes('All')) ||
                (provider === 'loadbalancing' && btn.textContent.includes('Load'))) {
                btn.classList.add('active');
            }
        });
    }
    
    // Update tab content visibility
    document.querySelectorAll('.provider-tab').forEach(tab => {
        tab.classList.remove('active');
    });
    const targetTab = document.getElementById(`${provider}-tab`);
    if (targetTab) {
        targetTab.classList.add('active');
    }
    
    // Load load balancing data if that tab is selected
    if (provider === 'loadbalancing') {
        loadLoadBalancingData();
    }
}

function loadRateLimit() {
    fetch('/api/rate-limit')
        .then(response => response.json())
        .then(data => {
            // Group workspaces by provider type
            const gmailWorkspaces = [];
            const mailgunWorkspaces = [];
            const mandrillWorkspaces = [];
            
            if (data.workspaces && data.workspaces.length > 0) {
                data.workspaces.forEach(ws => {
                    // Determine provider type based on configuration
                    if (ws.provider_type === 'gmail' || (ws.display_name && ws.display_name.toLowerCase().includes('gmail'))) {
                        gmailWorkspaces.push(ws);
                    } else if (ws.provider_type === 'mailgun' || (ws.display_name && ws.display_name.toLowerCase().includes('mailgun'))) {
                        mailgunWorkspaces.push(ws);
                    } else if (ws.provider_type === 'mandrill' || (ws.display_name && ws.display_name.toLowerCase().includes('mandrill'))) {
                        mandrillWorkspaces.push(ws);
                    } else {
                        // Default to Gmail for backward compatibility
                        gmailWorkspaces.push(ws);
                    }
                });
            }
            
            // Update metrics tab rate overview
            updateMetricsRateOverview(data, gmailWorkspaces, mailgunWorkspaces, mandrillWorkspaces);
            
            // Update provider tabs
            updateProviderTab('gmail-rate-limits', gmailWorkspaces);
            updateProviderTab('mailgun-rate-limits', mailgunWorkspaces);
            updateProviderTab('mandrill-rate-limits', mandrillWorkspaces);
            
            // Update All Providers summary
            const allRateLimitsDiv = document.getElementById('all-rate-limits');
            if (allRateLimitsDiv) {
                allRateLimitsDiv.innerHTML = `
                    <div class="rate-limit-stats">
                        <h4>System Overview</h4>
                        <p>Total Sent Today: <strong>${data.total_sent || 0}</strong></p>
                        <p>Active Workspaces: <strong>${data.workspace_count || 0}</strong></p>
                        <div style="margin-top: 20px;">
                            <h4>Provider Summary</h4>
                            <table class="provider-summary-table">
                                <tr>
                                    <td>Gmail Workspaces:</td>
                                    <td><strong>${gmailWorkspaces.length}</strong></td>
                                    <td>${gmailWorkspaces.map(ws => ws.workspace_id || ws.display_name).join(', ')}</td>
                                </tr>
                                <tr>
                                    <td>Mailgun Workspaces:</td>
                                    <td><strong>${mailgunWorkspaces.length}</strong></td>
                                    <td>${mailgunWorkspaces.map(ws => ws.workspace_id || ws.display_name).join(', ')}</td>
                                </tr>
                                <tr>
                                    <td>Mandrill Workspaces:</td>
                                    <td><strong>${mandrillWorkspaces.length}</strong></td>
                                    <td>${mandrillWorkspaces.map(ws => ws.workspace_id || ws.display_name).join(', ')}</td>
                                </tr>
                            </table>
                        </div>
                    </div>
                `;
            }
        })
        .catch(error => console.error('Error loading rate limit:', error));
}

// Update rate limit overview in metrics tab
function updateMetricsRateOverview(data, gmailWorkspaces, mailgunWorkspaces, mandrillWorkspaces) {
    const overviewDiv = document.getElementById('rate-limit-overview');
    if (!overviewDiv) return;
    
    const totalWorkspaces = gmailWorkspaces.length + mailgunWorkspaces.length + mandrillWorkspaces.length;
    const totalSent = data.total_sent || 0;
    
    // Calculate aggregated rate limit usage
    let totalCapacity = 0;
    let totalUsed = 0;
    
    [...gmailWorkspaces, ...mailgunWorkspaces, ...mandrillWorkspaces].forEach(ws => {
        totalCapacity += ws.workspace_limit || 2000;
        totalUsed += ws.workspace_sent || 0;
    });
    
    const overallPercentage = totalCapacity > 0 ? ((totalUsed / totalCapacity) * 100).toFixed(1) : 0;
    
    overviewDiv.innerHTML = `
        <div class="rate-overview-grid">
            <div class="rate-overview-card">
                <h4>Total Capacity</h4>
                <div class="rate-metric">
                    <span class="rate-value">${totalUsed}</span> / <span class="rate-limit">${totalCapacity}</span>
                </div>
                <div class="progress-bar">
                    <div class="progress-fill" style="width: ${overallPercentage}%"></div>
                </div>
                <small class="rate-percentage">${overallPercentage}% utilized</small>
            </div>
            <div class="rate-overview-card">
                <h4>Active Providers</h4>
                <div class="provider-counts">
                    <div class="provider-stat">
                        <span class="provider-icon">📬</span>
                        <span>Gmail: ${gmailWorkspaces.length}</span>
                    </div>
                    <div class="provider-stat">
                        <span class="provider-icon">📮</span>
                        <span>Mailgun: ${mailgunWorkspaces.length}</span>
                    </div>
                    <div class="provider-stat">
                        <span class="provider-icon">🐵</span>
                        <span>Mandrill: ${mandrillWorkspaces.length}</span>
                    </div>
                </div>
            </div>
        </div>
    `;
}

function updateProviderTab(elementId, workspaces) {
    const div = document.getElementById(elementId);
    if (!div) return;
    
    if (workspaces.length === 0) {
        div.innerHTML = '<p style="color: #666;">No workspaces configured for this provider</p>';
        return;
    }
    
    div.innerHTML = `
        <div class="rate-limit-stats">
            ${renderWorkspaceDetails(workspaces)}
        </div>
    `;
}

function renderWorkspaceDetails(workspaces) {
    if (!workspaces || workspaces.length === 0) {
        return '';
    }
    
    return workspaces.map(ws => {
        const totalLimit = ws.workspace_limit || 2000;
        const sent = ws.workspace_sent || 0;
        const remaining = ws.workspace_remaining || Math.max(0, totalLimit - sent);
        const percentage = totalLimit > 0 ? ((sent / totalLimit) * 100).toFixed(1) : 0;
        
        // Generate user details if available
        let userDetails = '';
        if (ws.users && Object.keys(ws.users).length > 0) {
            userDetails = '<div style="margin-top: 10px;"><h5 style="margin-bottom: 8px; color: #666;">User Rate Limits:</h5>';
            for (const [email, userData] of Object.entries(ws.users)) {
                const userPercentage = userData.limit > 0 ? ((userData.sent / userData.limit) * 100).toFixed(1) : 0;
                userDetails += `
                    <div style="margin-bottom: 8px; padding: 8px; background-color: #f9f9f9; border-radius: 4px; font-size: 12px;">
                        <div style="font-weight: 500;">${userData.email}</div>
                        <div>Sent: ${userData.sent} / Limit: ${userData.limit} (${userData.remaining} remaining)</div>
                        <div class="progress-bar" style="height: 15px; margin: 4px 0;">
                            <div class="progress-fill" style="width: ${userPercentage}%; background-color: #95a5a6;"></div>
                        </div>
                    </div>
                `;
            }
            userDetails += '</div>';
        }

        return `
            <div style="margin-top: 15px; padding: 10px; border: 1px solid #ddd; border-radius: 4px;">
                <h4>${ws.display_name || ws.workspace_id}</h4>
                <p><strong>Domains:</strong> ${ws.domains ? ws.domains.join(', ') : (ws.domain || 'N/A')}</p>
                <p>Sent: <strong>${sent}</strong> / Limit: <strong>${totalLimit}</strong> (${remaining} remaining)</p>
                <div class="progress-bar">
                    <div class="progress-fill" style="width: ${percentage}%"></div>
                </div>
                <p style="font-size: 12px; color: #666;">Resets daily at midnight UTC</p>
                ${userDetails}
            </div>
        `;
    }).join('');
}

function processQueue() {
    const statusElement = document.getElementById('process-status');
    const button = document.getElementById('process-btn');
    
    // Show processing status
    statusElement.textContent = '⏳ Processing...';
    statusElement.className = 'process-status processing';
    button.disabled = true;
    
    fetch('/api/process', { method: 'POST' })
        .then(response => response.json())
        .then(data => {
            console.log('Queue processing triggered:', data.message);
            
            // Show success status briefly
            statusElement.textContent = '✓ Processed';
            statusElement.className = 'process-status success';
            
            // Reload stats and messages after a short delay
            setTimeout(() => {
                loadStats();
                loadMessages();
                loadRateLimit();
                
                // Clear status after reload
                statusElement.textContent = '';
                statusElement.className = 'process-status';
                button.disabled = false;
            }, 1000);
        })
        .catch(error => {
            console.error('Error processing queue:', error);
            
            // Show error status
            statusElement.textContent = '✗ Error';
            statusElement.className = 'process-status error';
            
            // Clear error after 3 seconds
            setTimeout(() => {
                statusElement.textContent = '';
                statusElement.className = 'process-status';
                button.disabled = false;
            }, 3000);
        });
}

function loadLoadBalancingData() {
    // Load pools
    fetch('/api/loadbalancing/pools')
        .then(response => response.json())
        .then(data => {
            const poolsDiv = document.getElementById('lb-pools');
            if (!poolsDiv) return;
            
            if (!data.pools || data.pools.length === 0) {
                poolsDiv.innerHTML = '<p style="color: #666;">No load balancing pools configured</p>';
                return;
            }
            
            let poolsHTML = '<div class="lb-pools-grid">';
            data.pools.forEach(pool => {
                const statusClass = pool.enabled ? 'enabled' : 'disabled';
                const statusText = pool.enabled ? 'Active' : 'Disabled';
                
                poolsHTML += `
                    <div class="lb-pool-card ${statusClass}">
                        <h5>${escapeHtml(pool.name)}</h5>
                        <div class="lb-pool-info">
                            <p><strong>ID:</strong> ${escapeHtml(pool.id)}</p>
                            <p><strong>Strategy:</strong> ${escapeHtml(pool.strategy)}</p>
                            <p><strong>Status:</strong> <span class="status-${statusClass}">${statusText}</span></p>
                            <p><strong>Workspaces:</strong> ${pool.workspace_count}</p>
                            <p><strong>Selections (24h):</strong> ${pool.selection_count}</p>
                            <p><strong>Domains:</strong></p>
                            <ul class="domain-list">
                                ${pool.domain_patterns.map(d => `<li>${escapeHtml(d)}</li>`).join('')}
                            </ul>
                        </div>
                    </div>
                `;
            });
            poolsHTML += '</div>';
            poolsDiv.innerHTML = poolsHTML;
        })
        .catch(error => {
            console.error('Error loading pools:', error);
            document.getElementById('lb-pools').innerHTML = '<p style="color: #e74c3c;">Failed to load pools</p>';
        });
    
    // Load recent selections
    fetch('/api/loadbalancing/selections')
        .then(response => response.json())
        .then(data => {
            const tbody = document.getElementById('lb-selections-tbody');
            if (!tbody) return;
            
            if (!data.selections || data.selections.length === 0) {
                tbody.innerHTML = '<tr><td colspan="5" style="text-align: center; color: #666;">No recent selections</td></tr>';
                return;
            }
            
            let html = '';
            data.selections.forEach(sel => {
                html += `
                    <tr>
                        <td>${escapeHtml(sel.selected_at)}</td>
                        <td>${escapeHtml(sel.pool_name)}</td>
                        <td>${escapeHtml(sel.workspace_id)}</td>
                        <td>${escapeHtml(sel.sender_email)}</td>
                        <td>${escapeHtml(sel.capacity_score)}</td>
                    </tr>
                `;
            });
            tbody.innerHTML = html;
        })
        .catch(error => {
            console.error('Error loading selections:', error);
            document.getElementById('lb-selections-tbody').innerHTML = 
                '<tr><td colspan="5" style="text-align: center; color: #e74c3c;">Failed to load selections</td></tr>';
        });
}