// Lógica de Tabs
let currentArtifactsPage = 1;
let currentCommentsPage = 1;
const limit = 50;

function switchTab(tabId) {
    document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
    document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
    
    document.getElementById(tabId).classList.add('active');
    event.currentTarget.classList.add('active');

    if (window.statsInterval) {
        clearInterval(window.statsInterval);
        window.statsInterval = null;
    }

    if(tabId === 'artifacts' && currentArtifactsPage === 1) loadArtifacts();
    if(tabId === 'comments' && currentCommentsPage === 1) loadComments();
    if(tabId === 'stats') {
        loadStats();
        loadTopContributors();
        window.statsInterval = setInterval(() => {
            loadStats();
            loadTopContributors();
        }, 60000);
    }
}

// Utils
const formatDate = (dateStr) => {
    if(!dateStr) return '';
    const d = new Date(dateStr);
    return d.toLocaleDateString('pt-BR') + ' ' + d.toLocaleTimeString('pt-BR', {hour:'2-digit', minute:'2-digit'});
};

const escapeHtml = (unsafe) => {
    return (unsafe || '').replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
};

// Render Artifacts
async function loadArtifacts(page = 1) {
    currentArtifactsPage = page;
    const offset = (page - 1) * limit;
    const statusFilter = document.getElementById('status-filter') ? document.getElementById('status-filter').value : 'ALL';
    const sortByFilter = document.getElementById('sort-by-filter') ? document.getElementById('sort-by-filter').value : 'date';
    const sortFilter = document.getElementById('sort-filter') ? document.getElementById('sort-filter').value : 'desc';
    const minMembers = document.getElementById('min-members') ? document.getElementById('min-members').value : '';
    const maxMembers = document.getElementById('max-members') ? document.getElementById('max-members').value : '';
    const tagFilter = document.getElementById('tag-filter') ? document.getElementById('tag-filter').value.trim() : '';
    
    const grid = document.getElementById('artifacts-grid');

    if (grid) grid.innerHTML = '<div class="loading"><i class="fa-solid fa-circle-notch fa-spin"></i> Carregando servidores...</div>';
    
    try {
        let url = `/api/artifacts?limit=${limit}&offset=${offset}&status=${statusFilter}&sort_by=${sortByFilter}&sort=${sortFilter}`;
        if (minMembers) url += `&min_members=${minMembers}`;
        if (maxMembers) url += `&max_members=${maxMembers}`;
        if (tagFilter) url += `&tag=${encodeURIComponent(tagFilter)}`;
        const res = await fetch(url);
        const data = await res.json();
        renderArtifactsGrid(data, grid);
        renderPagination('artifacts-pagination', data.page, data.total_pages, loadArtifacts);
    } catch (err) {
        if (grid) grid.innerHTML = `<div class="empty-state">Erro ao carregar dados: ${err.message}</div>`;
    }
}

async function searchArtifacts(query, page = 1) {
    currentArtifactsPage = page;
    const offset = (page - 1) * limit;
    const statusFilter = document.getElementById('status-filter') ? document.getElementById('status-filter').value : 'ALL';
    const sortByFilter = document.getElementById('sort-by-filter') ? document.getElementById('sort-by-filter').value : 'date';
    const sortFilter = document.getElementById('sort-filter') ? document.getElementById('sort-filter').value : 'desc';
    const minMembers = document.getElementById('min-members') ? document.getElementById('min-members').value : '';
    const maxMembers = document.getElementById('max-members') ? document.getElementById('max-members').value : '';
    const tagFilter = document.getElementById('tag-filter') ? document.getElementById('tag-filter').value.trim() : '';
    
    const grid = document.getElementById('artifacts-grid');

    if (grid) grid.innerHTML = '<div class="loading"><i class="fa-solid fa-circle-notch fa-spin"></i> Buscando servidores...</div>';
    
    try {
        let url = `/api/search?q=${encodeURIComponent(query)}&limit=${limit}&offset=${offset}&status=${statusFilter}&sort_by=${sortByFilter}&sort=${sortFilter}`;
        if (minMembers) url += `&min_members=${minMembers}`;
        if (maxMembers) url += `&max_members=${maxMembers}`;
        if (tagFilter) url += `&tag=${encodeURIComponent(tagFilter)}`;

        const res = await fetch(url);
        const data = await res.json();
        renderArtifactsGrid(data, grid);
        renderPagination('artifacts-pagination', data.page, data.total_pages, (newPage) => searchArtifacts(query, newPage));
    } catch (err) {
        if (grid) grid.innerHTML = `<div class="empty-state">Erro na busca: ${err.message}</div>`;
    }
}

function renderArtifactsGrid(data, grid) {
    if(!grid) return;
    if(!data || !data.items || data.items.length === 0) {
        grid.innerHTML = `<div class="empty-state"><i class="fa-solid fa-ghost fa-3x" style="margin-bottom:1rem;opacity:0.5"></i><br>Nenhum servidor encontrado.</div>`;
        return;
    }

    grid.innerHTML = data.items.map(item => {
        const status = (item.discord_status || 'UNKNOWN').toUpperCase();
        let statusClass = 'status-unknown';
        if (status === 'ACTIVE' || status === 'VALID') statusClass = 'status-valid';
        else if (status === 'INVALID' || status === 'EXPIRED') statusClass = 'status-invalid';
        else if (status === 'RATE_LIMITED') statusClass = 'status-rate-limited';

        const serverName = item.discord_server_name || 'Desconhecido';
        
        let iconUrl = null;
        if (item.discord_icon) {
            if (item.discord_icon.startsWith('http')) {
                iconUrl = item.discord_icon;
            } else {
                iconUrl = `https://cdn.discordapp.com/icons/${item.discord_server_id}/${item.discord_icon}.png`;
            }
        }
        const avatarUrl = item.avatar_url || 'https://ui-avatars.com/api/?name=' + item.author_id + '&background=random';
        
        let tagsHtml = '';
        if (item.tags) {
            item.tags.split(',').forEach(tag => {
                if (tag.trim() === '') return;
                tagsHtml += `<span style="background: rgba(88, 101, 242, 0.2); border: 1px solid #5865f2; color: #fff; padding: 2px 8px; border-radius: 12px; font-size: 0.7rem; margin-right: 4px;">${tag.trim()}</span>`;
            });
        }
        
        const codes = item.discord_invite_codes ? item.discord_invite_codes.split(',') : [];
        const inviteUrl = codes.length > 0 ? `https://discord.gg/${codes[0]}` : '#';
        const videoUrl = item.source_url;
        const date = new Date(item.processed_at).toLocaleString('pt-BR');
        
        let codesHtml = '';
        const visibleCodes = codes.slice(0, 5);
        const hiddenCodes = codes.slice(5);

        visibleCodes.forEach(code => {
            codesHtml += `<span style="background: rgba(255,255,255,0.1); padding: 2px 6px; border-radius: 4px; font-size: 0.75rem; color: #ddd; margin-right: 4px; margin-bottom: 4px; display: inline-block; border: 1px solid rgba(255,255,255,0.1);"><i class="fa-solid fa-link"></i> ${code}</span>`;
        });

        if (hiddenCodes.length > 0) {
            let hiddenHtml = '';
            hiddenCodes.forEach(code => {
                hiddenHtml += `<span style="background: rgba(255,255,255,0.1); padding: 2px 6px; border-radius: 4px; font-size: 0.75rem; color: #ddd; margin-right: 4px; margin-bottom: 4px; display: inline-block; border: 1px solid rgba(255,255,255,0.1);"><i class="fa-solid fa-link"></i> ${code}</span>`;
            });
            const hiddenId = 'hidden-codes-' + Math.random().toString(36).substr(2, 9);
            codesHtml += `
                <div id="${hiddenId}" style="display: none; margin-top: 4px;">
                    ${hiddenHtml}
                </div>
                <button onclick="const el = document.getElementById('${hiddenId}'); if(el.style.display==='none'){el.style.display='block';this.innerText='Esconder'}else{el.style.display='none';this.innerHTML='+ ${hiddenCodes.length} códigos...'}" style="background: none; border: none; color: #5865f2; font-size: 0.75rem; cursor: pointer; padding: 0; margin-top: 4px;">
                    + ${hiddenCodes.length} códigos...
                </button>
            `;
        }

        return `
            <div class="card">
                <div class="card-header">
                    <div class="discord-info">
                        ${iconUrl ? `<img src="${iconUrl}" class="discord-icon" onerror="this.onerror=null; this.src='https://cdn.discordapp.com/embed/avatars/0.png';">` : '<div class="discord-icon" style="background: var(--surface-light); display: flex; align-items: center; justify-content: center;"><i class="fa-brands fa-discord" style="color: #5865f2; font-size: 1.5rem;"></i></div>'}
                        <div class="server-details">
                            <h3 class="server-name">${serverName}</h3>
                            <div class="member-count">
                                <i class="fa-solid fa-users"></i> ${item.discord_member_count || 0} membros
                                <span style="color: #f39c12; margin-left: 8px;"><i class="fa-solid fa-fire"></i> Apareceu ${item.mentions_count || 1}x</span>
                            </div>
                            <div style="margin-top: 4px;">${tagsHtml}</div>
                        </div>
                    </div>
                    <span class="status-badge ${statusClass}">${status}</span>
                </div>
                
                <div style="margin: 10px 15px;">
                    ${codesHtml}
                </div>

                <div class="author-section">
                    <img src="${avatarUrl}" class="author-avatar" onerror="this.src='https://ui-avatars.com/api/?name=${item.author_id}&background=random'">
                    <div class="author-info">
                        <div class="author-name">@${item.author_id}</div>
                        <div class="post-date">Último envio em ${date}</div>
                    </div>
                </div>

                <div class="action-row" style="margin: 0 15px 15px 15px;">
                    <a href="${inviteUrl}" target="_blank" class="btn btn-primary">
                        <i class="fa-solid fa-right-to-bracket"></i> Entrar
                    </a>
                    <a href="${videoUrl}" target="_blank" class="btn btn-secondary">
                        <i class="fa-brands fa-tiktok"></i> Vídeo Original
                    </a>
                    <button onclick="editTags(${item.id}, '${item.tags || ''}')" class="btn btn-secondary" style="background: var(--surface-light);">
                        <i class="fa-solid fa-tags"></i> Tags
                    </button>
                </div>
            </div>
        `;
    }).join('');
}

window.editTags = async (id, currentTags) => {
    const newTags = prompt("Editar tags (separadas por vírgula):", currentTags);
    if (newTags !== null) {
        try {
            await fetch(`/api/artifacts/${id}/tags`, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ tags: newTags })
            });
            loadArtifacts(currentArtifactsPage);
        } catch (err) {
            console.error(err);
            alert("Erro ao salvar tags.");
        }
    }
};

// Render Comments
async function loadComments(page = 1) {
    currentCommentsPage = page;
    const offset = (page - 1) * limit;
    const grid = document.getElementById('comments-grid');

    if (grid) grid.innerHTML = '<div class="loading"><i class="fa-solid fa-circle-notch fa-spin"></i> Carregando comentários...</div>';
    
    try {
        const res = await fetch(`/api/comments?limit=${limit}&offset=${offset}`);
        const data = await res.json();
        
        if (!grid) return;

        if(!data || !data.items || data.items.length === 0) {
            grid.innerHTML = `<div class="empty-state"><i class="fa-solid fa-comment-slash fa-3x" style="margin-bottom:1rem;opacity:0.5"></i><br>Nenhum comentário coletado ainda.</div>`;
            return;
        }

        grid.innerHTML = data.items.map(item => {
            const avatarUrl = item.avatar_url || 'https://ui-avatars.com/api/?name=' + item.nickname + '&background=random';
            const videoUrl = `https://www.tiktok.com/@${item.unique_id}/video/${item.aweme_id}#comment-${item.cid}`;

            return `
            <div class="card">
                <div class="author-section" style="background:transparent; padding:0; border:none;">
                    <img src="${avatarUrl}" class="author-avatar" onerror="this.src='https://ui-avatars.com/api/?name=${item.nickname}&background=random'">
                    <div>
                        <div class="author-name">${escapeHtml(item.nickname)} <span style="color:var(--text-muted); font-size:0.8rem">@${item.unique_id || item.nickname}</span></div>
                        <div class="author-sub">${formatDate(item.created_at)}</div>
                    </div>
                </div>
                
                <div class="comment-text">
                    ${escapeHtml(item.text)}
                </div>

                <div style="margin-top:auto; display:flex; justify-content:space-between; align-items:center;">
                    <div class="comment-stats">
                        <span><i class="fa-solid fa-heart"></i> ${item.digg_count || 0}</span>
                    </div>
                    <a href="${videoUrl}" target="_blank" class="btn btn-secondary" style="padding: 0.5rem 1rem;">
                        Ver no TikTok
                    </a>
                </div>
            </div>
            `;
        }).join('');
        
        renderPagination('comments-pagination', data.page, data.total_pages, loadComments);
    } catch (err) {
        if (grid) grid.innerHTML = `<div class="empty-state">Erro ao carregar dados: ${err.message}</div>`;
    }
}

// Render Pagination
function renderPagination(containerId, currentPage, totalPages, onPageChange) {
    const container = document.getElementById(containerId);
    if (!container) return;

    if (!totalPages || totalPages <= 1) {
        container.innerHTML = '';
        return;
    }

    let html = '';

    // Botão Anterior (Primeira e Anterior)
    html += `<button onclick="window.pendingPageChange(1, 1)" ${currentPage === 1 ? 'disabled' : ''}>&laquo;</button>`;
    html += `<button onclick="window.pendingPageChange(1, ${currentPage - 1})" ${currentPage === 1 ? 'disabled' : ''}>&lsaquo;</button>`;

    // Lógica da janela de ±2
    const windowSize = 2;
    let startPage = Math.max(1, currentPage - windowSize);
    let endPage = Math.min(totalPages, currentPage + windowSize);

    if (startPage > 1) {
        html += `<button onclick="window.pendingPageChange(1, 1)">1</button>`;
        if (startPage > 2) html += `<span style="color: var(--text-muted); margin: 0 0.5rem;">...</span>`;
    }

    for (let i = startPage; i <= endPage; i++) {
        html += `<button onclick="window.pendingPageChange(1, ${i})" class="${i === currentPage ? 'active' : ''}">${i}</button>`;
    }

    if (endPage < totalPages) {
        if (endPage < totalPages - 1) html += `<span style="color: var(--text-muted); margin: 0 0.5rem;">...</span>`;
        html += `<button onclick="window.pendingPageChange(1, ${totalPages})">${totalPages}</button>`;
    }

    // Botão Próxima (Próxima e Última)
    html += `<button onclick="window.pendingPageChange(1, ${currentPage + 1})" ${currentPage === totalPages ? 'disabled' : ''}>&rsaquo;</button>`;
    html += `<button onclick="window.pendingPageChange(1, ${totalPages})" ${currentPage === totalPages ? 'disabled' : ''}>&raquo;</button>`;

    // Input "Ir para página"
    html += `
        <div style="display:flex; align-items:center; gap:0.5rem; margin-left: 1rem;">
            <span style="color: var(--text-muted); font-size: 0.9rem;">Ir para página:</span>
            <input type="number" min="1" max="${totalPages}" style="width: 60px; background: rgba(0,0,0,0.2); border: 1px solid rgba(255,255,255,0.1); color: white; padding: 4px 8px; border-radius: 4px; outline: none;"
                onchange="window.pendingPageChange(1, parseInt(this.value))">
        </div>
    `;

    // Store callback to be used by window.pendingPageChange
    window.pendingPageChangeCallback = onPageChange;
    window.pendingPageChangeTotalPages = totalPages;

    container.innerHTML = html;
}

window.pendingPageChange = function(_, newPage) {
    if (newPage < 1 || newPage > window.pendingPageChangeTotalPages) return;
    if (window.pendingPageChangeCallback) {
        window.pendingPageChangeCallback(newPage);
    }
}

// Initial Load e Listeners
// Export Data
window.exportData = (format) => {
    const statusFilter = document.getElementById('status-filter') ? document.getElementById('status-filter').value : 'ALL';
    const minMembers = document.getElementById('min-members') ? document.getElementById('min-members').value : '';
    const maxMembers = document.getElementById('max-members') ? document.getElementById('max-members').value : '';
    const tagFilter = document.getElementById('tag-filter') ? document.getElementById('tag-filter').value.trim() : '';
    const searchInput = document.getElementById('search-input');
    const q = searchInput ? searchInput.value.trim() : '';

    let url = `/api/export?format=${format}&status=${statusFilter}`;
    if (minMembers) url += `&min_members=${minMembers}`;
    if (maxMembers) url += `&max_members=${maxMembers}`;
    if (tagFilter) url += `&tag=${encodeURIComponent(tagFilter)}`;
    if (q.length >= 2) url += `&q=${encodeURIComponent(q)}`;

    window.open(url, '_blank');
};

// CountUp animation
function animateValue(obj, start, end, duration) {
    let startTimestamp = null;
    const step = (timestamp) => {
        if (!startTimestamp) startTimestamp = timestamp;
        const progress = Math.min((timestamp - startTimestamp) / duration, 1);
        obj.innerHTML = Math.floor(progress * (end - start) + start);
        if (progress < 1) {
            window.requestAnimationFrame(step);
        }
    };
    window.requestAnimationFrame(step);
}

// Load Stats
async function loadStats() {
    try {
        const res = await fetch('/api/stats');
        const data = await res.json();
        
        const summary = document.getElementById('stats-summary');
        if (summary) {
            summary.innerHTML = `
                <div class="card" style="text-align: center; background: linear-gradient(135deg, rgba(88, 101, 242, 0.1) 0%, rgba(25, 28, 36, 0.6) 100%);">
                    <h3 style="color: var(--text-muted); font-size: 0.9rem; text-transform: uppercase; letter-spacing: 1px;">Total de Servidores</h3>
                    <div class="stat-value" style="font-size: 2.5rem; color: #5865f2; font-weight: 800; text-shadow: 0 0 20px rgba(88, 101, 242, 0.3); margin-top: 0.5rem;" id="stat-total">0</div>
                </div>
                <div class="card" style="text-align: center; background: linear-gradient(135deg, rgba(243, 156, 18, 0.1) 0%, rgba(25, 28, 36, 0.6) 100%);">
                    <h3 style="color: var(--text-muted); font-size: 0.9rem; text-transform: uppercase; letter-spacing: 1px;">Últimas 24h</h3>
                    <div class="stat-value" style="font-size: 2.5rem; color: #f39c12; font-weight: 800; text-shadow: 0 0 20px rgba(243, 156, 18, 0.3); margin-top: 0.5rem;" id="stat-24h">0</div>
                </div>
                <div class="card" style="text-align: center; background: linear-gradient(135deg, rgba(46, 204, 113, 0.1) 0%, rgba(25, 28, 36, 0.6) 100%);">
                    <h3 style="color: var(--text-muted); font-size: 0.9rem; text-transform: uppercase; letter-spacing: 1px;">Ativos</h3>
                    <div class="stat-value" style="font-size: 2.5rem; color: #2ecc71; font-weight: 800; text-shadow: 0 0 20px rgba(46, 204, 113, 0.3); margin-top: 0.5rem;" id="stat-active">0</div>
                </div>
                <div class="card" style="text-align: center; background: linear-gradient(135deg, rgba(231, 76, 60, 0.1) 0%, rgba(25, 28, 36, 0.6) 100%);">
                    <h3 style="color: var(--text-muted); font-size: 0.9rem; text-transform: uppercase; letter-spacing: 1px;">Rate Limited</h3>
                    <div class="stat-value" style="font-size: 2.5rem; color: #e74c3c; font-weight: 800; text-shadow: 0 0 20px rgba(231, 76, 60, 0.3); margin-top: 0.5rem;" id="stat-rl">0</div>
                </div>
            `;

            animateValue(document.getElementById('stat-total'), 0, data.total, 1000);
            animateValue(document.getElementById('stat-24h'), 0, data.total_24h, 1000);
            animateValue(document.getElementById('stat-active'), 0, data.status.active, 1000);
            animateValue(document.getElementById('stat-rl'), 0, data.status.rate_limited, 1000);
        }

        const tagsList = document.getElementById('top-tags-list');
        if (tagsList) {
            if (!data.top_tags || data.top_tags.length === 0) {
                tagsList.innerHTML = '<div class="empty-state" style="padding: 2rem;">Nenhuma tag encontrada ainda.</div>';
            } else {
                tagsList.innerHTML = data.top_tags.map(t => `
                    <div class="card" style="display: flex; justify-content: space-between; padding: 1rem;">
                        <span style="font-weight: bold;"><i class="fa-solid fa-tag" style="color: #5865f2;"></i> ${t.tag}</span>
                        <span style="background: rgba(88, 101, 242, 0.2); color: #fff; padding: 2px 10px; border-radius: 12px; font-weight: bold;">${t.count}</span>
                    </div>
                `).join('');
            }
        }
    } catch (err) {
        console.error(err);
    }
}

// Load Top Contributors
async function loadTopContributors() {
    try {
        const res = await fetch('/api/stats/top-contributors');
        const data = await res.json();
        
        const list = document.getElementById('top-contributors-list');
        if (!list) return;

        if (!data || data.length === 0) {
            list.innerHTML = 'Nenhum contribuidor encontrado.';
            return;
        }

        list.innerHTML = data.map((c, i) => {
            const avatarUrl = c.avatar_url || 'https://ui-avatars.com/api/?name=' + c.author_id + '&background=random';
            let medal = '';
            if (i === 0) medal = '<i class="fa-solid fa-medal" style="color: gold;"></i> ';
            if (i === 1) medal = '<i class="fa-solid fa-medal" style="color: silver;"></i> ';
            if (i === 2) medal = '<i class="fa-solid fa-medal" style="color: #cd7f32;"></i> ';
            
            // Usa o unique_id se existir (ex: @user123), caso contrário cai pro author_id
            const usernameToLink = c.unique_id ? c.unique_id : c.author_id;
            const tiktokUsername = encodeURIComponent(usernameToLink.trim());

            return `
                <div class="card" style="display: flex; align-items: center; justify-content: space-between; padding: 1rem; gap: 1rem; flex-wrap: wrap;">
                    <div style="display: flex; align-items: center; gap: 1rem;">
                        <span style="font-size: 1.2rem; font-weight: bold; width: 30px; text-align: center;">${i+1}º</span>
                        <img src="${avatarUrl}" style="width: 40px; height: 40px; border-radius: 50%;" onerror="this.src='https://ui-avatars.com/api/?name=${c.author_id}&background=random'">
                        <div>
                            <div style="font-weight: bold; font-size: 1.1rem;">${medal}${c.author_id}</div>
                            ${c.unique_id ? `<div style="font-size: 0.8rem; color: var(--text-muted);">@${c.unique_id}</div>` : ''}
                        </div>
                    </div>
                    <div style="display: flex; align-items: center; gap: 1rem; margin-left: auto;">
                        <div style="font-size: 1.5rem; font-weight: bold; color: #5865f2; text-align: right;">
                            ${c.total} <span style="font-size: 0.8rem; color: var(--text-muted);">contribuições</span>
                        </div>
                        <a href="https://www.tiktok.com/@${tiktokUsername}" target="_blank" class="btn btn-secondary" style="padding: 0.5rem 0.8rem; font-size: 0.85rem;" title="Ver Perfil no TikTok">
                            <i class="fa-brands fa-tiktok"></i> Ver Perfil
                        </a>
                    </div>
                </div>
            `;
        }).join('');
    } catch (err) {
        console.error(err);
    }
}

function applyFilters() {
    const searchInput = document.getElementById('search-input');
    const query = searchInput ? searchInput.value.trim() : '';
    if (query.length >= 2) {
        searchArtifacts(query, 1);
    } else {
        loadArtifacts(1);
    }
}
window.applyFilters = applyFilters;

if (typeof window !== 'undefined') {
    window.onload = () => {
        loadArtifacts();

        // Listeners
        const searchInput = document.getElementById('search-input');
        if (searchInput) {
            let searchTimeout;
            searchInput.addEventListener('input', (e) => {
                clearTimeout(searchTimeout);
                searchTimeout = setTimeout(() => {
                    const query = e.target.value.trim();
                    if (query.length >= 2) {
                        searchArtifacts(query);
                    } else {
                        loadArtifacts(1); // volta pra listagem normal
                    }
                }, 300);
            });
        }

        const minMembers = document.getElementById('min-members');
        const maxMembers = document.getElementById('max-members');
        const tagFilter = document.getElementById('tag-filter');
        const statusFilter = document.getElementById('status-filter');
        const sortByFilter = document.getElementById('sort-by-filter');
        const sortFilter = document.getElementById('sort-filter');

        if (minMembers) minMembers.addEventListener('input', applyFilters);
        if (maxMembers) maxMembers.addEventListener('input', applyFilters);
        if (statusFilter) statusFilter.addEventListener('change', applyFilters);
        if (sortByFilter) sortByFilter.addEventListener('change', applyFilters);
        if (sortFilter) sortFilter.addEventListener('change', applyFilters);
        if (tagFilter) tagFilter.addEventListener('input', () => {
            clearTimeout(window.tagFilterTimeout);
            window.tagFilterTimeout = setTimeout(applyFilters, 300);
        });
    };
}
