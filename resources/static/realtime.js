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
    const source = new EventSource('/stream/' + roomid);
    source.addEventListener('message', newChatMessage, false);
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
