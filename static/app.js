let sessionId = localStorage.getItem('sessionId');
let currentUser = null;
let ws = null;
let currentGame = null;
let selectedSquare = null;
let pendingInvite = null;
let playerColor = null;

const pieceUnicode = {
    'P': '\u2659', 'R': '\u2656', 'N': '\u2658', 'B': '\u2657', 'Q': '\u2655', 'K': '\u2654',
    'p': '\u265F', 'r': '\u265C', 'n': '\u265E', 'b': '\u265D', 'q': '\u265B', 'k': '\u265A'
};

async function init() {
    if (sessionId) {
        const response = await fetch('/api/me', {
            headers: { 'X-Session-ID': sessionId }
        });
        const data = await response.json();
        if (data.success) {
            currentUser = data.user;
            showMainContainer();
            connectWebSocket();
            loadLeaderboard();
        } else {
            localStorage.removeItem('sessionId');
            sessionId = null;
        }
    }
}

function showLogin() {
    document.getElementById('login-form').style.display = 'block';
    document.getElementById('register-form').style.display = 'none';
}

function showRegister() {
    document.getElementById('login-form').style.display = 'none';
    document.getElementById('register-form').style.display = 'block';
}

async function login() {
    const username = document.getElementById('login-username').value;
    const password = document.getElementById('login-password').value;

    const response = await fetch('/api/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password })
    });

    const data = await response.json();
    showAuthMessage(data.message, data.success);

    if (data.success) {
        sessionId = data.session_id;
        currentUser = data.user;
        localStorage.setItem('sessionId', sessionId);
        showMainContainer();
        connectWebSocket();
        loadLeaderboard();
    }
}

async function register() {
    const username = document.getElementById('register-username').value;
    const email = document.getElementById('register-email').value;
    const password = document.getElementById('register-password').value;

    const response = await fetch('/api/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, email, password })
    });

    const data = await response.json();
    showAuthMessage(data.message, data.success);

    if (data.success) {
        showLogin();
    }
}

async function logout() {
    await fetch('/api/logout', {
        method: 'POST',
        headers: { 'X-Session-ID': sessionId }
    });

    if (ws) {
        ws.close();
    }

    localStorage.removeItem('sessionId');
    sessionId = null;
    currentUser = null;
    currentGame = null;

    document.getElementById('main-container').style.display = 'none';
    document.getElementById('admin-panel').style.display = 'none';
    document.getElementById('auth-container').style.display = 'flex';
}

function showAuthMessage(message, success) {
    const el = document.getElementById('auth-message');
    el.textContent = message;
    el.className = 'message ' + (success ? 'success' : 'error');
    el.style.display = 'block';
}

function showMainContainer() {
    document.getElementById('auth-container').style.display = 'none';
    document.getElementById('main-container').style.display = 'block';
    document.getElementById('current-user').textContent = currentUser.username;
    document.getElementById('user-rating').textContent = 'Rating: ' + currentUser.rating;

    if (currentUser.is_admin) {
        document.getElementById('admin-btn').style.display = 'inline-block';
    }

    renderEmptyBoard();
}

var wsReconnectTimer = null;

function connectWebSocket() {
    if (wsReconnectTimer) {
        clearTimeout(wsReconnectTimer);
        wsReconnectTimer = null;
    }

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    ws = new WebSocket(protocol + '//' + window.location.host + '/ws?session=' + sessionId);

    ws.onopen = function() {
        console.log('WebSocket connected');
        // Check for active game now that WebSocket is ready
        checkActiveGame();
    };

    ws.onmessage = function(event) {
        console.log('WebSocket message:', event.data);
        const msg = JSON.parse(event.data);
        handleWebSocketMessage(msg);
    };

    ws.onerror = function(error) {
        console.error('WebSocket error:', error);
    };

    ws.onclose = function(event) {
        console.log('WebSocket closed, code:', event.code);
        // Only reconnect if not a normal closure and user is still logged in
        if (sessionId && event.code !== 1000) {
            wsReconnectTimer = setTimeout(connectWebSocket, 5000);
        }
    };
}

function handleWebSocketMessage(msg) {
    switch (msg.type) {
        case 'online_users':
            updateOnlineUsers(msg.payload);
            break;
        case 'game_invite':
            showGameInvite(msg.payload);
            break;
        case 'invite_declined':
            alert(msg.payload.message);
            break;
        case 'game_start':
            startGame(msg.payload);
            break;
        case 'game_update':
            updateGame(msg.payload);
            break;
        case 'game_chat':
            addChatMessage(msg.payload);
            break;
        case 'direct_message':
            addDirectMessage(msg.payload);
            break;
        case 'error':
            alert(msg.payload.error);
            break;
    }
}

function updateOnlineUsers(users) {
    const list = document.getElementById('users-list');
    list.innerHTML = '';

    console.log('Updating online users:', users, 'currentGame:', currentGame);

    if (!users || users.length === 0) {
        list.innerHTML = '<li>No other users online</li>';
        return;
    }

    users.forEach(user => {
        if (user.id === currentUser.id) return;

        const li = document.createElement('li');
        li.className = user.in_game ? 'in-game' : '';

        const nameSpan = document.createElement('span');
        nameSpan.textContent = user.username;
        li.appendChild(nameSpan);

        console.log('User:', user.username, 'in_game:', user.in_game, 'currentGame:', currentGame);

        if (!user.in_game && !currentGame) {
            const btn = document.createElement('button');
            btn.className = 'invite-btn';
            btn.textContent = 'Invite';
            btn.onclick = () => sendInvite(user.id);
            li.appendChild(btn);
        } else if (user.in_game) {
            const status = document.createElement('span');
            status.textContent = '(in game)';
            status.style.fontSize = '12px';
            status.style.color = '#888';
            li.appendChild(status);
        }

        list.appendChild(li);
    });
}

function sendInvite(userId) {
    console.log('Sending invite to user:', userId);
    console.log('WebSocket state:', ws.readyState);
    if (ws.readyState !== WebSocket.OPEN) {
        console.error('WebSocket not open!');
        return;
    }
    const msg = JSON.stringify({
        type: 'invite',
        payload: { to_user_id: userId }
    });
    console.log('Sending message:', msg);
    ws.send(msg);
}

function showGameInvite(invite) {
    pendingInvite = invite;
    document.getElementById('invite-message').textContent =
        invite.from_username + ' has invited you to play chess!';
    document.getElementById('invite-modal').style.display = 'flex';
}

function acceptInvite() {
    if (!pendingInvite) return;

    ws.send(JSON.stringify({
        type: 'accept_invite',
        payload: { from_user_id: pendingInvite.from_user_id }
    }));

    document.getElementById('invite-modal').style.display = 'none';
    pendingInvite = null;
}

function declineInvite() {
    if (!pendingInvite) return;

    ws.send(JSON.stringify({
        type: 'decline_invite',
        payload: { from_user_id: pendingInvite.from_user_id }
    }));

    document.getElementById('invite-modal').style.display = 'none';
    pendingInvite = null;
}

function startGame(data) {
    currentGame = {
        id: data.game_id,
        white_user_id: data.white_user_id,
        black_user_id: data.black_user_id,
        fen: data.fen,
        moves: []
    };

    playerColor = currentUser.id === data.white_user_id ? 'white' : 'black';

    document.getElementById('no-game-message').style.display = 'none';
    document.getElementById('game-info').style.display = 'flex';
    document.getElementById('game-controls').style.display = 'block';

    fetchGameInfo();
    renderBoard(data.fen);
}

async function fetchGameInfo() {
    const response = await fetch('/api/game?id=' + currentGame.id);
    const data = await response.json();
    if (data.success) {
        const opponentName = currentUser.id === data.game.white_user_id
            ? data.black_username
            : data.white_username;
        document.getElementById('opponent-name').textContent =
            'Playing vs ' + opponentName + ' (' + playerColor + ')';
    }
}

async function checkActiveGame() {
    const response = await fetch('/api/game/active', {
        headers: { 'X-Session-ID': sessionId }
    });
    const data = await response.json();
    if (data.success && data.game) {
        currentGame = data.game;
        playerColor = currentUser.id === data.game.white_user_id ? 'white' : 'black';

        document.getElementById('no-game-message').style.display = 'none';
        document.getElementById('game-info').style.display = 'flex';
        document.getElementById('game-controls').style.display = 'block';

        const opponentName = currentUser.id === data.game.white_user_id
            ? data.black_username
            : data.white_username;
        document.getElementById('opponent-name').textContent =
            'Playing vs ' + opponentName + ' (' + playerColor + ')';

        renderBoard(data.game.fen);
        loadGameChat();

        // Tell server we're rejoining this game (if WS is ready, otherwise onopen will handle it)
        if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({
                type: 'rejoin_game',
                payload: { game_id: data.game.id }
            }));
            console.log('Sent rejoin_game for game', data.game.id);
        }
    }
}

function updateGame(data) {
    if (!currentGame || currentGame.id !== data.game_id) return;

    if (data.fen) {
        currentGame.fen = data.fen;
        currentGame.moves.push(data.move);
        renderBoard(data.fen);
    }

    let status = '';
    if (data.status === 'check') {
        status = 'Check!';
    } else if (data.status === 'checkmate') {
        status = data.winner_id === currentUser.id ? 'Checkmate! You win!' : 'Checkmate! You lose!';
        endGameUI();
    } else if (data.status === 'stalemate') {
        status = 'Stalemate! Draw!';
        endGameUI();
    } else if (data.status === 'resigned') {
        status = data.resigned === currentUser.id ? 'You resigned!' : 'Opponent resigned! You win!';
        endGameUI();
    }

    document.getElementById('game-status').textContent = status;
}

function endGameUI() {
    currentGame = null;
    playerColor = null;
    selectedSquare = null;

    setTimeout(() => {
        document.getElementById('game-info').style.display = 'none';
        document.getElementById('game-controls').style.display = 'none';
        document.getElementById('no-game-message').style.display = 'block';
        document.getElementById('game-status').textContent = '';
        document.getElementById('chat-messages').innerHTML = '';
        renderEmptyBoard();
        loadLeaderboard();
    }, 3000);
}

function renderEmptyBoard() {
    renderBoard('rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1');
}

function renderBoard(fen) {
    const board = document.getElementById('chess-board');
    board.innerHTML = '';

    const position = parseFEN(fen);
    const flipped = playerColor === 'black';

    for (let r = 0; r < 8; r++) {
        for (let c = 0; c < 8; c++) {
            const row = flipped ? 7 - r : r;
            const col = flipped ? 7 - c : c;

            const square = document.createElement('div');
            square.className = 'square ' + ((row + col) % 2 === 0 ? 'light' : 'dark');
            square.dataset.row = row;
            square.dataset.col = col;

            const piece = position[row][col];
            if (piece) {
                square.textContent = pieceUnicode[piece];
                if (piece === piece.toUpperCase()) {
                    square.classList.add('white-piece');
                } else {
                    square.classList.add('black-piece');
                }
            }

            if (currentGame) {
                square.onclick = () => handleSquareClick(row, col);
            }

            board.appendChild(square);
        }
    }
}

function parseFEN(fen) {
    const position = Array(8).fill(null).map(() => Array(8).fill(null));
    const rows = fen.split(' ')[0].split('/');

    for (let r = 0; r < 8; r++) {
        let col = 0;
        for (const ch of rows[r]) {
            if (ch >= '1' && ch <= '8') {
                col += parseInt(ch);
            } else {
                position[r][col] = ch;
                col++;
            }
        }
    }

    return position;
}

function handleSquareClick(row, col) {
    if (!currentGame) return;

    const isWhiteTurn = currentGame.fen.split(' ')[1] === 'w';
    const isMyTurn = (playerColor === 'white' && isWhiteTurn) || (playerColor === 'black' && !isWhiteTurn);

    if (!isMyTurn) return;

    const position = parseFEN(currentGame.fen);
    const piece = position[row][col];

    if (selectedSquare) {
        const fromSquare = coordsToSquare(selectedSquare.row, selectedSquare.col);
        const toSquare = coordsToSquare(row, col);

        if (selectedSquare.row === row && selectedSquare.col === col) {
            selectedSquare = null;
            renderBoard(currentGame.fen);
            return;
        }

        let moveStr = fromSquare + toSquare;

        const fromPiece = position[selectedSquare.row][selectedSquare.col];
        if ((fromPiece === 'P' && row === 0) || (fromPiece === 'p' && row === 7)) {
            moveStr += 'q';
        }

        console.log('Sending move:', moveStr);
        ws.send(JSON.stringify({
            type: 'move',
            payload: { game_id: currentGame.id, move: moveStr }
        }));

        selectedSquare = null;
    } else if (piece) {
        const isWhitePiece = piece === piece.toUpperCase();
        if ((playerColor === 'white' && isWhitePiece) || (playerColor === 'black' && !isWhitePiece)) {
            selectedSquare = { row, col };
            renderBoard(currentGame.fen);
            highlightSquare(row, col);
        }
    }
}

function highlightSquare(row, col) {
    const flipped = playerColor === 'black';
    const displayRow = flipped ? 7 - row : row;
    const displayCol = flipped ? 7 - col : col;
    const index = displayRow * 8 + displayCol;

    const squares = document.querySelectorAll('.square');
    if (squares[index]) {
        squares[index].classList.add('selected');
    }
}

function coordsToSquare(row, col) {
    return String.fromCharCode(97 + col) + (8 - row);
}

function resignGame() {
    if (!currentGame) return;

    if (confirm('Are you sure you want to resign?')) {
        ws.send(JSON.stringify({
            type: 'resign',
            payload: { game_id: currentGame.id }
        }));
    }
}

function handleChatKeypress(event) {
    if (event.key === 'Enter') {
        sendChat();
    }
}

function sendChat() {
    if (!currentGame) return;

    const input = document.getElementById('chat-input');
    const content = input.value.trim();
    if (!content) return;

    ws.send(JSON.stringify({
        type: 'chat',
        payload: { game_id: currentGame.id, content: content }
    }));

    input.value = '';
}

function addChatMessage(data) {
    const container = document.getElementById('chat-messages');
    const div = document.createElement('div');
    div.className = 'chat-message';
    div.innerHTML = '<span class="username">' + data.from_username + ':</span> <span class="content">' + escapeHtml(data.content) + '</span>';
    container.appendChild(div);
    container.scrollTop = container.scrollHeight;
}

async function loadGameChat() {
    if (!currentGame) return;

    const response = await fetch('/api/game/messages?game_id=' + currentGame.id);
    const data = await response.json();
    if (data.success && data.messages) {
        const container = document.getElementById('chat-messages');
        container.innerHTML = '';
        data.messages.forEach(msg => {
            addChatMessage({
                from_username: msg.from_username,
                content: msg.content
            });
        });
    }
}

function handleDMKeypress(event) {
    if (event.key === 'Enter') {
        sendDM();
    }
}

function sendDM() {
    const input = document.getElementById('dm-input');
    const content = input.value.trim();
    if (!content) return;

    ws.send(JSON.stringify({
        type: 'direct_message',
        payload: { content: content }
    }));

    input.value = '';
}

function addDirectMessage(data) {
    const container = document.getElementById('dm-messages');
    const div = document.createElement('div');
    div.className = 'chat-message';
    div.innerHTML = '<span class="username">' + data.from_username + ':</span> <span class="content">' + escapeHtml(data.content) + '</span>';
    container.appendChild(div);
    container.scrollTop = container.scrollHeight;
}

async function loadLeaderboard() {
    const response = await fetch('/api/leaderboard');
    const data = await response.json();
    if (data.success && data.users) {
        const list = document.getElementById('leaderboard-list');
        list.innerHTML = '';
        data.users.slice(0, 10).forEach((user, index) => {
            const li = document.createElement('li');
            li.innerHTML = '<span>' + user.username + '</span><span>' + user.rating + '</span>';
            list.appendChild(li);
        });
    }
}

async function showAdminPanel() {
    document.getElementById('main-container').style.display = 'none';
    document.getElementById('admin-panel').style.display = 'block';
    loadPendingUsers();
    loadSettings();
    loadAllUsers();
}

function hideAdminPanel() {
    document.getElementById('admin-panel').style.display = 'none';
    document.getElementById('main-container').style.display = 'block';
}

async function loadPendingUsers() {
    const response = await fetch('/api/admin/pending', {
        headers: { 'X-Session-ID': sessionId }
    });
    const data = await response.json();
    if (data.success && data.users) {
        const list = document.getElementById('pending-users-list');
        list.innerHTML = '';

        if (data.users.length === 0) {
            list.innerHTML = '<li>No pending users</li>';
            return;
        }

        data.users.forEach(user => {
            const li = document.createElement('li');
            li.innerHTML = '<span>' + user.username + ' (' + user.email + ')</span><div>' +
                '<button class="approve" onclick="approveUser(' + user.id + ')">Approve</button>' +
                '<button class="reject" onclick="rejectUser(' + user.id + ')">Reject</button></div>';
            list.appendChild(li);
        });
    }
}

async function approveUser(userId) {
    console.log('Approving user:', userId);
    const response = await fetch('/api/admin/approve', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'X-Session-ID': sessionId
        },
        body: JSON.stringify({ user_id: userId })
    });
    const data = await response.json();
    console.log('Approve response:', data);
    if (data.success) {
        console.log('Refreshing pending users list');
        loadPendingUsers();
        loadAllUsers();
    }
}

async function rejectUser(userId) {
    const response = await fetch('/api/admin/reject', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'X-Session-ID': sessionId
        },
        body: JSON.stringify({ user_id: userId })
    });
    const data = await response.json();
    if (data.success) {
        loadPendingUsers();
        loadAllUsers();
    }
}

async function loadSettings() {
    const response = await fetch('/api/admin/settings', {
        headers: { 'X-Session-ID': sessionId }
    });
    const data = await response.json();
    if (data.success) {
        document.getElementById('profanity-filter-toggle').checked = data.profanity_filter;
        document.getElementById('profanity-auto-warn-toggle').checked = data.profanity_auto_warn;
    }
}

async function toggleProfanityFilter() {
    const enabled = document.getElementById('profanity-filter-toggle').checked;
    await fetch('/api/admin/settings', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'X-Session-ID': sessionId
        },
        body: JSON.stringify({ profanity_filter: enabled })
    });
}

async function toggleProfanityAutoWarn() {
    const enabled = document.getElementById('profanity-auto-warn-toggle').checked;
    await fetch('/api/admin/settings', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'X-Session-ID': sessionId
        },
        body: JSON.stringify({ profanity_auto_warn: enabled })
    });
}

async function loadAllUsers() {
    const response = await fetch('/api/admin/users', {
        headers: { 'X-Session-ID': sessionId }
    });
    const data = await response.json();
    if (data.success && data.users) {
        const tbody = document.getElementById('all-users-body');
        tbody.innerHTML = '';
        data.users.forEach(user => {
            const tr = document.createElement('tr');
            const statusClass = user.approved ? 'status-approved' : 'status-pending';
            const statusText = user.approved ? 'Approved' : 'Pending';
            tr.innerHTML =
                '<td>' + user.username + (user.is_admin ? ' (Admin)' : '') + '</td>' +
                '<td>' + user.rating + '</td>' +
                '<td>' + user.games_played + '</td>' +
                '<td>' + user.wins + '/' + user.losses + '/' + user.draws + '</td>' +
                '<td class="' + statusClass + '">' + statusText + '</td>';
            tbody.appendChild(tr);
        });
    }
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

init();
