let currentPage = 1;
const pageSize = 20;
let currentStatus = 'all';

document.addEventListener('DOMContentLoaded', function() {
    loadStats();
    loadMessages();
    loadRateLimit();
    
    document.getElementById('refresh-btn').addEventListener('click', function() {
        loadStats();
        loadMessages();
        loadRateLimit();
    });
    
    document.getElementById('status-filter').addEventListener('change', function(e) {
        currentStatus = e.target.value;
        currentPage = 1;
        loadMessages();
    });
    
    document.getElementById('prev-btn').addEventListener('click', function() {
        if (currentPage > 1) {
            currentPage--;
            loadMessages();
        }
    });
    
    document.getElementById('next-btn').addEventListener('click', function() {
        currentPage++;
        loadMessages();
    });
    
    document.querySelector('.close').addEventListener('click', closeModal);
    
    window.addEventListener('click', function(e) {
        if (e.target === document.getElementById('message-modal')) {
            closeModal();
        }
    });
    
    setInterval(function() {
        loadStats();
        loadMessages();
        loadRateLimit();
    }, 10000);
});

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

function loadRateLimit() {
    fetch('/api/rate-limit')
        .then(response => response.json())
        .then(data => {
            const rateLimitDiv = document.getElementById('rate-limit-info');
            if (rateLimitDiv) {
                let workspaceDetails = '';
                if (data.workspaces && data.workspaces.length > 0) {
                    workspaceDetails = data.workspaces.map(ws => {
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
                                <p><strong>Domain:</strong> ${ws.domain || 'N/A'}</p>
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
                
                rateLimitDiv.innerHTML = `
                    <div class="rate-limit-stats">
                        <h3>Rate Limit Status</h3>
                        <p>Total Sent Today: <strong>${data.total_sent || 0}</strong></p>
                        <p>Active Workspaces: <strong>${data.workspace_count || 0}</strong></p>
                        ${workspaceDetails}
                    </div>
                `;
            }
        })
        .catch(error => console.error('Error loading rate limit:', error));
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