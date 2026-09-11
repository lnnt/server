function StartRealtime(roomid) {
    StartSSE(roomid);
    StartForm();
}

function StartForm() {
    const messageInput = document.getElementById('chat-message');
    const form = document.getElementById('chat-form');
    if (!messageInput || !form) return;

    messageInput.focus();

    form.addEventListener('submit', async (e) => {
        e.preventDefault();
        const formData = new FormData(form);

        try {
            await fetch(form.action, {
                method: 'POST',
                body: formData
            });
            messageInput.value = '';
            messageInput.focus();
        } catch (err) {
            console.error('Failed to send message:', err);
        }
    });
}

function StartSSE(roomid) {
    if (!window.EventSource) {
        alert('EventSource is not supported in this browser');
        return;
    }
    const nick = new URLSearchParams(location.search).get('nick');
    let url = '/stream/' + encodeURIComponent(roomid);
    if (nick) url += '?nick=' + encodeURIComponent(nick);

    const source = new EventSource(url);
    source.addEventListener('message', newChatMessage, false);
    source.addEventListener('presence', onPresence, false);
}

function onPresence(e) {
    const users = JSON.parse(e.data);
    const list = document.getElementById('users');
    if (!list) return;

    list.innerHTML = '';
    let total = 0;
    users.forEach(u => {
        total += u.count;

        const li = document.createElement('li');
        li.className = 'list-group-item d-flex justify-content-between align-items-center py-1';

        const name = document.createElement('span');
        name.textContent = u.nick;
        li.appendChild(name);

        if (u.count > 1) {
            const badge = document.createElement('span');
            badge.className = 'badge text-bg-secondary';
            badge.textContent = '×' + u.count;
            li.appendChild(badge);
        }

        list.appendChild(li);
    });

    const count = document.getElementById('users-count');
    if (count) count.textContent = '(' + total + ')';
}

function newChatMessage(e) {
    const data = JSON.parse(e.data);
    const nick = data.nick;
    const message = data.message;
    const style = rowStyle(nick);

    const tr = document.createElement('tr');
    tr.className = style;
    tr.innerHTML = `<td>${escapeHtml(nick)}</td><td>${escapeHtml(message)}</td>`;

    const chat = document.getElementById('chat');
    const scroll = document.getElementById('chat-scroll');

    if (chat) chat.appendChild(tr);
    if (scroll) scroll.scrollTop = scroll.scrollHeight;
}

function rowStyle(nick) {
    const classes = ['table-active', 'table-success', 'table-info', 'table-warning', 'table-danger'];
    return classes[hashCode(nick) % 5];
}

function hashCode(s) {
    return Math.abs(
        s.split('').reduce((a, b) => {
            a = ((a << 5) - a) + b.charCodeAt(0);
            return a & a;
        }, 0)
    );
}

function escapeHtml(str) {
    const map = {
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#39;',
        '/': '&#x2F;'
    };
    return String(str).replace(/[&<>"'\/]/g, s => map[s]);
}

window.StartRealtime = StartRealtime;
