// Lógica de Tabs
let currentArtifactsPage = 1;
let currentCommentsPage = 1;
const limit = 50;

function switchTab(tabId) {
    document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
    document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
    
    document.getElementById(tabId).classList.add('active');
    event.currentTarget.classList.add('active');

    if(tabId === 'artifacts' && currentArtifactsPage === 1) loadArtifacts();
    if(tabId === 'comments' && currentCommentsPage === 1) loadComments();
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
    const minMembers = document.getElementById('min-members') ? document.getElementById('min-members').value : '';
    const maxMembers = document.getElementById('max-members') ? document.getElementById('max-members').value : '';
    
    const grid = document.getElementById('artifacts-grid');

    if (grid) grid.innerHTML = '<div class="loading"><i class="fa-solid fa-circle-notch fa-spin"></i> Carregando servidores...</div>';
    
    try {
        let url = `/api/artifacts?limit=${limit}&offset=${offset}&status=${statusFilter}`;
        if (minMembers) url += `&min_members=${minMembers}`;
        if (maxMembers) url += `&max_members=${maxMembers}`;
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
    const grid = document.getElementById('artifacts-grid');

    if (grid) grid.innerHTML = '<div class="loading"><i class="fa-solid fa-circle-notch fa-spin"></i> Buscando servidores...</div>';
    
    try {
        const res = await fetch(`/api/search?q=${encodeURIComponent(query)}&limit=${limit}&offset=${offset}`);
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
                </div>
            </div>
        `;
    }).join('');
}

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

    // Botão Anterior
    html += `<button onclick="window.pendingPageChange(1, ${currentPage - 1})" ${currentPage === 1 ? 'disabled' : ''}>&laquo;</button>`;

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

    // Botão Próxima
    html += `<button onclick="window.pendingPageChange(1, ${currentPage + 1})" ${currentPage === totalPages ? 'disabled' : ''}>&raquo;</button>`;

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
        if (minMembers) {
            minMembers.addEventListener('input', () => {
                const query = searchInput ? searchInput.value.trim() : '';
                if (query.length >= 2) searchArtifacts(query, 1);
                else loadArtifacts(1);
            });
        }
        if (maxMembers) {
            maxMembers.addEventListener('input', () => {
                const query = searchInput ? searchInput.value.trim() : '';
                if (query.length >= 2) searchArtifacts(query, 1);
                else loadArtifacts(1);
            });
        }
    };
}
