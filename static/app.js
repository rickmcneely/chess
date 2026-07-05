let sessionId = localStorage.getItem('sessionId');
let currentUser = null;
let ws = null;
let currentGame = null;
let selectedSquare = null;
let pendingInvite = null;
let playerColor = null;
let clockInterval = null;
let observingGame = null;
let playerPerspective = false;
let view3D = false;

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

    // Refresh active games when connected
    setTimeout(refreshActiveGames, 500);
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
            handleObserverGameUpdate(msg.payload);
            break;
        case 'clock_update':
            handleClockUpdate(msg.payload);
            break;
        case 'game_chat':
            addChatMessage(msg.payload);
            break;
        case 'direct_message':
            addDirectMessage(msg.payload);
            break;
        case 'active_games':
            updateActiveGamesList(msg.payload);
            break;
        case 'observe_game_state':
            startObserving(msg.payload);
            break;
        case 'observe_game_ended':
            handleObserveGameEnded(msg.payload);
            break;
        case 'observer_count':
            updateObserverCount(msg.payload);
            break;
        case 'messages_read':
            handleMessagesRead(msg.payload);
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

function playAI() {
    if (ws.readyState !== WebSocket.OPEN) {
        console.error('WebSocket not open!');
        return;
    }
    const difficulty = parseInt(document.getElementById('ai-difficulty').value);
    const timeControlSelect = document.getElementById('ai-time-control');
    const [timeControlMS, incrementMS] = timeControlSelect.value.split('|').map(Number);

    ws.send(JSON.stringify({
        type: 'play_ai',
        payload: {
            difficulty: difficulty,
            time_control_ms: timeControlMS,
            increment_ms: incrementMS
        }
    }));
    console.log('Starting AI game with difficulty:', difficulty, 'time control:', timeControlMS, '+', incrementMS);
}

function sendInvite(userId) {
    console.log('Sending invite to user:', userId);
    console.log('WebSocket state:', ws.readyState);
    if (ws.readyState !== WebSocket.OPEN) {
        console.error('WebSocket not open!');
        return;
    }
    const timeControlSelect = document.getElementById('time-control');
    const [timeControlMS, incrementMS] = timeControlSelect.value.split('|').map(Number);

    const msg = JSON.stringify({
        type: 'invite',
        payload: {
            to_user_id: userId,
            time_control_ms: timeControlMS,
            increment_ms: incrementMS
        }
    });
    console.log('Sending message:', msg);
    ws.send(msg);
}

function showGameInvite(invite) {
    pendingInvite = invite;
    document.getElementById('invite-message').textContent =
        invite.from_username + ' has invited you to play chess!';

    // Show time control info
    const timeControlDisplay = document.getElementById('invite-time-control-display');
    if (invite.time_control_ms > 0) {
        timeControlDisplay.textContent = 'Time Control: ' + invite.time_control_name;
        timeControlDisplay.style.display = 'block';
    } else {
        timeControlDisplay.textContent = 'Time Control: No Clock';
        timeControlDisplay.style.display = 'block';
    }

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
        moves: [],
        vs_ai: data.vs_ai || false,
        time_control_ms: data.time_control_ms || 0,
        increment_ms: data.increment_ms || 0,
        white_time_ms: data.white_time_ms || 0,
        black_time_ms: data.black_time_ms || 0
    };

    playerColor = data.player_color || (currentUser.id === data.white_user_id ? 'white' : 'black');

    document.getElementById('no-game-message').style.display = 'none';
    document.getElementById('game-info').style.display = 'flex';
    document.getElementById('game-controls').style.display = 'block';

    // Show clocks if timed game
    if (data.time_control_ms > 0) {
        document.getElementById('clock-container').style.display = 'flex';
        document.getElementById('clock-container-bottom').style.display = 'flex';
        updateClockDisplays();
    } else {
        document.getElementById('clock-container').style.display = 'none';
        document.getElementById('clock-container-bottom').style.display = 'none';
    }

    // If we have username info already (AI games), use it
    if (data.white_username && data.black_username) {
        const opponentName = currentUser.id === data.white_user_id
            ? data.black_username
            : data.white_username;
        let gameInfo = 'Playing vs ' + opponentName + ' (' + playerColor + ')';
        if (data.time_control_name) {
            gameInfo += ' [' + data.time_control_name + ']';
        }
        document.getElementById('opponent-name').textContent = gameInfo;
    } else {
        fetchGameInfo();
    }

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

        let gameInfo = 'Playing vs ' + opponentName + ' (' + playerColor + ')';
        if (data.game.time_control_ms > 0) {
            const mins = data.game.time_control_ms / 60000;
            const incSec = data.game.increment_ms / 1000;
            gameInfo += ' [' + mins + '+' + incSec + ']';
        }
        document.getElementById('opponent-name').textContent = gameInfo;

        // Initialize clock times from game state
        if (data.game.time_control_ms > 0) {
            currentGame.time_control_ms = data.game.time_control_ms;
            currentGame.increment_ms = data.game.increment_ms || 0;
            // Use the calculated remaining times from the server (accounts for elapsed time)
            currentGame.white_time_ms = data.white_time_remaining !== undefined
                ? data.white_time_remaining
                : (data.game.white_time_remaining || data.game.time_control_ms);
            currentGame.black_time_ms = data.black_time_remaining !== undefined
                ? data.black_time_remaining
                : (data.game.black_time_remaining || data.game.time_control_ms);

            document.getElementById('clock-container').style.display = 'flex';
            document.getElementById('clock-container-bottom').style.display = 'flex';
            updateClockDisplays();
        }

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

    // Update clocks if present
    if (data.white_time_ms !== undefined) {
        currentGame.white_time_ms = data.white_time_ms;
    }
    if (data.black_time_ms !== undefined) {
        currentGame.black_time_ms = data.black_time_ms;
    }
    if (currentGame.time_control_ms > 0) {
        updateClockDisplays();
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
    } else if (data.status === 'draw_50move') {
        status = 'Draw by 50-move rule!';
        endGameUI();
    } else if (data.status === 'draw_repetition') {
        status = 'Draw by threefold repetition!';
        endGameUI();
    } else if (data.status === 'draw_insufficient') {
        status = 'Draw by insufficient material!';
        endGameUI();
    } else if (data.status === 'resigned') {
        status = data.resigned === currentUser.id ? 'You resigned!' : 'Opponent resigned! You win!';
        endGameUI();
    } else if (data.status === 'timeout') {
        status = data.timeout === currentUser.id ? 'Time out! You lose!' : 'Opponent ran out of time! You win!';
        endGameUI();
    }

    document.getElementById('game-status').textContent = status;
}

function endGameUI() {
    currentGame = null;
    playerColor = null;
    selectedSquare = null;

    // Clear clock interval if running
    if (clockInterval) {
        clearInterval(clockInterval);
        clockInterval = null;
    }

    setTimeout(() => {
        document.getElementById('game-info').style.display = 'none';
        document.getElementById('game-controls').style.display = 'none';
        document.getElementById('clock-container').style.display = 'none';
        document.getElementById('clock-container-bottom').style.display = 'none';
        document.getElementById('no-game-message').style.display = 'block';
        document.getElementById('game-status').textContent = '';
        document.getElementById('chat-messages').innerHTML = '';
        renderEmptyBoard();
        loadLeaderboard();
    }, 3000);
}

// Clock handling functions
function updateClockDisplays() {
    if (!currentGame || !currentGame.time_control_ms) return;

    const playerClock = document.getElementById('player-clock');
    const opponentClock = document.getElementById('opponent-clock');

    let playerTimeMs, opponentTimeMs;
    if (playerColor === 'white') {
        playerTimeMs = currentGame.white_time_ms;
        opponentTimeMs = currentGame.black_time_ms;
    } else {
        playerTimeMs = currentGame.black_time_ms;
        opponentTimeMs = currentGame.white_time_ms;
    }

    playerClock.textContent = formatTime(playerTimeMs);
    opponentClock.textContent = formatTime(opponentTimeMs);

    // Determine whose turn it is based on FEN
    const isWhiteTurn = currentGame.fen.split(' ')[1] === 'w';
    const activeColor = isWhiteTurn ? 'white' : 'black';

    // Highlight active clock and apply warning colors
    updateClockStyle(playerClock, playerTimeMs, playerColor === activeColor);
    updateClockStyle(opponentClock, opponentTimeMs, playerColor !== activeColor);
}

function updateClockStyle(clockElement, timeMs, isActive) {
    // Reset classes
    clockElement.classList.remove('clock-active', 'clock-low', 'clock-critical');

    if (isActive) {
        clockElement.classList.add('clock-active');
    }

    if (timeMs <= 10000) {
        clockElement.classList.add('clock-critical');
    } else if (timeMs <= 30000) {
        clockElement.classList.add('clock-low');
    }
}

function formatTime(ms) {
    if (ms <= 0) {
        return '0:00';
    }
    const totalSeconds = Math.floor(ms / 1000);
    const mins = Math.floor(totalSeconds / 60);
    const secs = totalSeconds % 60;
    return mins + ':' + secs.toString().padStart(2, '0');
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
    const input = document.getElementById('chat-input');
    const content = input.value.trim();
    if (!content) return;

    // Check if it's a DM (starts with @)
    if (content.startsWith('@')) {
        ws.send(JSON.stringify({
            type: 'direct_message',
            payload: { content: content }
        }));
        // Show own DM in chat
        addChatMessage({
            from_username: currentUser.username,
            content: content,
            is_dm: true
        });
        input.value = '';
    } else if (currentGame) {
        // Send to current game opponent
        ws.send(JSON.stringify({
            type: 'chat',
            payload: { game_id: currentGame.id, content: content }
        }));
        // Show own message immediately (server also echoes, we'll dedupe)
        addChatMessage({
            from_username: currentUser.username,
            content: content,
            is_own: true,
            msg_id: Date.now() // For deduplication
        });
        input.value = '';
    } else {
        // Not in a game - show hint
        updateChatHint('Start a game or use @username for DM');
    }
}

var recentMessages = new Set();

function addChatMessage(data) {
    // Deduplicate messages (for when server echoes back our own message)
    const msgKey = data.from_username + ':' + data.content;
    if (data.from_username === currentUser.username && !data.is_own && !data.is_dm) {
        // This is a server echo of our own game chat message - skip it
        if (recentMessages.has(msgKey)) {
            return;
        }
    }

    // Track recent messages for deduplication
    if (data.is_own) {
        recentMessages.add(msgKey);
        // Clear after 5 seconds
        setTimeout(() => recentMessages.delete(msgKey), 5000);
    }

    const container = document.getElementById('chat-messages');
    const div = document.createElement('div');
    div.className = 'chat-message';

    if (data.is_dm) {
        div.classList.add('dm-message');
    }
    if (data.stored) {
        div.classList.add('stored-message');
    }
    if (data.from_username === currentUser.username) {
        div.classList.add('own-message');
    }

    let prefix = '';
    if (data.stored) {
        prefix = '<span class="stored-badge">STORED</span> ';
    }

    div.innerHTML = prefix + '<span class="username">' + escapeHtml(data.from_username) + ':</span> <span class="content">' + escapeHtml(data.content) + '</span>';
    container.appendChild(div);
    container.scrollTop = container.scrollHeight;
}

function addDirectMessage(data) {
    // Add DMs to the unified chat window
    addChatMessage({
        from_username: data.from_username,
        from_user_id: data.from_user_id,
        content: data.content,
        is_dm: true,
        stored: data.stored,
        message_id: data.message_id
    });

    // Mark as read after displaying
    if (data.from_user_id && data.from_user_id !== currentUser.id) {
        markMessagesRead(data.from_user_id);
    }
}

function markMessagesRead(fromUserID) {
    if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({
            type: 'mark_read',
            payload: { from_user_id: fromUserID }
        }));
    }
}

function handleMessagesRead(data) {
    // Show read indicator - update UI to show messages were read
    const indicator = document.createElement('div');
    indicator.className = 'read-indicator';
    indicator.textContent = data.reader_username + ' read your messages';
    document.getElementById('chat-messages').appendChild(indicator);

    // Auto-remove after 3 seconds
    setTimeout(() => indicator.remove(), 3000);
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

function updateChatHint(message) {
    const hint = document.getElementById('chat-hint');
    hint.textContent = message;
    hint.style.display = 'block';
    setTimeout(() => {
        hint.style.display = 'none';
    }, 3000);
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
            const deleteBtn = user.is_admin ? '' :
                '<button class="delete-btn" onclick="deleteUser(' + user.id + ', \'' + escapeHtml(user.username) + '\')">Delete</button>';
            tr.innerHTML =
                '<td>' + user.username + (user.is_admin ? ' (Admin)' : '') + '</td>' +
                '<td>' + user.rating + '</td>' +
                '<td>' + user.games_played + '</td>' +
                '<td>' + user.wins + '/' + user.losses + '/' + user.draws + '</td>' +
                '<td class="' + statusClass + '">' + statusText + '</td>' +
                '<td>' + deleteBtn + '</td>';
            tbody.appendChild(tr);
        });
    }
}

async function deleteUser(userId, username) {
    if (!confirm('Are you sure you want to delete user "' + username + '"? This will also delete all their games and messages.')) {
        return;
    }

    const response = await fetch('/api/admin/delete', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'X-Session-ID': sessionId
        },
        body: JSON.stringify({ user_id: userId })
    });
    const data = await response.json();
    if (data.success) {
        loadAllUsers();
    } else {
        alert(data.message || 'Failed to delete user');
    }
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Observer functionality
function refreshActiveGames() {
    if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'get_active_games', payload: {} }));
    }
}

function updateActiveGamesList(games) {
    const list = document.getElementById('active-games-list');
    list.innerHTML = '';

    if (!games || games.length === 0) {
        list.innerHTML = '<li class="no-games">No active games</li>';
        return;
    }

    games.forEach(game => {
        const li = document.createElement('li');
        li.className = 'active-game-item';

        const info = document.createElement('span');
        info.className = 'game-info';
        info.textContent = game.white_username + ' vs ' + game.black_username;
        if (game.time_control_ms) {
            const mins = game.time_control_ms / 60000;
            const inc = game.increment_ms / 1000;
            info.textContent += ' [' + mins + '+' + inc + ']';
        }
        info.textContent += ' (' + game.move_count + ' moves)';
        li.appendChild(info);

        if (game.observer_count > 0) {
            const observers = document.createElement('span');
            observers.className = 'observer-count';
            observers.textContent = game.observer_count + ' watching';
            li.appendChild(observers);
        }

        const watchBtn = document.createElement('button');
        watchBtn.className = 'watch-btn';
        watchBtn.textContent = 'Watch';
        watchBtn.onclick = () => observeGame(game.game_id);
        li.appendChild(watchBtn);

        list.appendChild(li);
    });
}

function observeGame(gameId) {
    if (currentGame) {
        alert('You are currently in a game');
        return;
    }

    ws.send(JSON.stringify({
        type: 'observe_game',
        payload: { game_id: gameId }
    }));
}

function startObserving(data) {
    observingGame = {
        id: data.game_id,
        white_user_id: data.white_user_id,
        black_user_id: data.black_user_id,
        white_username: data.white_username,
        black_username: data.black_username,
        fen: data.fen,
        moves: data.moves || [],
        time_control_ms: data.time_control_ms || 0,
        increment_ms: data.increment_ms || 0,
        white_time_ms: data.white_time_ms || 0,
        black_time_ms: data.black_time_ms || 0
    };

    // Reset view options
    playerPerspective = false;
    view3D = false;
    document.getElementById('player-perspective').checked = false;
    document.getElementById('view-3d').checked = false;

    document.getElementById('no-game-message').style.display = 'none';
    document.getElementById('game-info').style.display = 'flex';
    document.getElementById('observer-controls').style.display = 'flex';
    document.getElementById('game-controls').style.display = 'none';

    document.getElementById('opponent-name').textContent =
        'Watching: ' + data.white_username + ' vs ' + data.black_username;

    // Show clocks if timed
    if (data.time_control_ms > 0) {
        document.getElementById('clock-container').style.display = 'flex';
        document.getElementById('clock-container-bottom').style.display = 'flex';
        updateObserverClocks();
    } else {
        document.getElementById('clock-container').style.display = 'none';
        document.getElementById('clock-container-bottom').style.display = 'none';
    }

    renderObserverBoard();
}

function handleObserverGameUpdate(data) {
    if (!observingGame || observingGame.id !== data.game_id) return;

    if (data.fen) {
        observingGame.fen = data.fen;
        if (data.move) {
            observingGame.moves.push(data.move);
        }
        renderObserverBoard();
    }

    if (data.white_time_ms !== undefined) {
        observingGame.white_time_ms = data.white_time_ms;
    }
    if (data.black_time_ms !== undefined) {
        observingGame.black_time_ms = data.black_time_ms;
    }
    if (observingGame.time_control_ms > 0) {
        updateObserverClocks();
    }

    // Handle game end
    if (data.status === 'checkmate' || data.status === 'stalemate' ||
        data.status === 'resigned' || data.status === 'timeout' ||
        (data.status && data.status.startsWith('draw_'))) {
        let statusText = '';
        if (data.status === 'checkmate') {
            statusText = 'Checkmate!';
        } else if (data.status === 'stalemate') {
            statusText = 'Stalemate - Draw!';
        } else if (data.status === 'resigned') {
            statusText = 'Game ended by resignation';
        } else if (data.status === 'timeout') {
            statusText = 'Game ended on time';
        } else {
            statusText = 'Draw';
        }
        document.getElementById('game-status').textContent = statusText;
    }
}

function handleObserveGameEnded(data) {
    if (!observingGame || observingGame.id !== data.game_id) return;
    stopObservingUI();
    alert('The game you were watching has ended');
}

function stopObserving() {
    if (!observingGame) return;

    ws.send(JSON.stringify({ type: 'stop_observing', payload: {} }));
    stopObservingUI();
}

function stopObservingUI() {
    observingGame = null;
    playerPerspective = false;
    view3D = false;

    document.getElementById('game-info').style.display = 'none';
    document.getElementById('observer-controls').style.display = 'none';
    document.getElementById('clock-container').style.display = 'none';
    document.getElementById('clock-container-bottom').style.display = 'none';
    document.getElementById('no-game-message').style.display = 'block';
    document.getElementById('game-status').textContent = '';
    document.getElementById('chess-board').classList.remove('board-3d');

    renderEmptyBoard();
    refreshActiveGames();
}

function updateObserverCount(data) {
    // Could update a display of observer count if desired
    console.log('Game', data.game_id, 'has', data.observer_count, 'observers');
}

function handleClockUpdate(data) {
    // Handle for playing
    if (currentGame && currentGame.id === data.game_id) {
        currentGame.white_time_ms = data.white_time_ms;
        currentGame.black_time_ms = data.black_time_ms;
        currentGame.active_color = data.active_color;
        updateClockDisplays();
    }

    // Handle for observing
    if (observingGame && observingGame.id === data.game_id) {
        observingGame.white_time_ms = data.white_time_ms;
        observingGame.black_time_ms = data.black_time_ms;
        observingGame.active_color = data.active_color;
        updateObserverClocks();
    }
}

function updateObserverClocks() {
    if (!observingGame || !observingGame.time_control_ms) return;

    const topClock = document.getElementById('opponent-clock');
    const bottomClock = document.getElementById('player-clock');

    // In observer mode, determine which player is at top/bottom based on perspective
    let topTimeMs, bottomTimeMs, topColor, bottomColor;

    if (playerPerspective) {
        // Get current turn to show that player's perspective
        const isWhiteTurn = observingGame.fen.split(' ')[1] === 'w';
        if (isWhiteTurn) {
            // Show from white's perspective (black at top)
            topTimeMs = observingGame.black_time_ms;
            bottomTimeMs = observingGame.white_time_ms;
            topColor = 'black';
            bottomColor = 'white';
        } else {
            // Show from black's perspective (white at top)
            topTimeMs = observingGame.white_time_ms;
            bottomTimeMs = observingGame.black_time_ms;
            topColor = 'white';
            bottomColor = 'black';
        }
    } else {
        // Standard view: white at bottom
        topTimeMs = observingGame.black_time_ms;
        bottomTimeMs = observingGame.white_time_ms;
        topColor = 'black';
        bottomColor = 'white';
    }

    topClock.textContent = formatTime(topTimeMs);
    bottomClock.textContent = formatTime(bottomTimeMs);

    // Determine active color
    const isWhiteTurn = observingGame.fen.split(' ')[1] === 'w';
    const activeColor = isWhiteTurn ? 'white' : 'black';

    updateClockStyle(topClock, topTimeMs, topColor === activeColor);
    updateClockStyle(bottomClock, bottomTimeMs, bottomColor === activeColor);
}

function renderObserverBoard() {
    if (!observingGame) return;

    const board = document.getElementById('chess-board');
    board.innerHTML = '';

    // Apply 3D class if enabled
    if (view3D) {
        board.classList.add('board-3d');
    } else {
        board.classList.remove('board-3d');
    }

    const position = parseFEN(observingGame.fen);

    // Determine board orientation
    let flipped = false;
    if (playerPerspective) {
        // Flip to show current player's perspective
        const isWhiteTurn = observingGame.fen.split(' ')[1] === 'w';
        flipped = !isWhiteTurn; // Flip if it's black's turn
    }

    for (let r = 0; r < 8; r++) {
        for (let c = 0; c < 8; c++) {
            const row = flipped ? 7 - r : r;
            const col = flipped ? 7 - c : c;

            const square = document.createElement('div');
            square.className = 'square ' + ((row + col) % 2 === 0 ? 'light' : 'dark');

            if (view3D) {
                square.classList.add('square-3d');
            }

            const piece = position[row][col];
            if (piece) {
                const pieceSpan = document.createElement('span');
                pieceSpan.className = 'piece';
                if (view3D) {
                    pieceSpan.classList.add('piece-3d');
                }
                pieceSpan.textContent = pieceUnicode[piece];
                if (piece === piece.toUpperCase()) {
                    square.classList.add('white-piece');
                } else {
                    square.classList.add('black-piece');
                }
                square.appendChild(pieceSpan);
            }

            board.appendChild(square);
        }
    }
}

function togglePlayerPerspective() {
    playerPerspective = document.getElementById('player-perspective').checked;
    if (observingGame) {
        renderObserverBoard();
        updateObserverClocks();
    }
}

function toggle3DView() {
    view3D = document.getElementById('view-3d').checked;
    if (observingGame) {
        renderObserverBoard();
    }
}

init();
