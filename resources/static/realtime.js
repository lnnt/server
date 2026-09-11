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
            const resp = await fetch(form.action, {
                method: 'POST',
                body: formData
            });
            if (!resp.ok) {
                const err = await resp.json().catch(() => null);
                // 发送失败(比如 @ 的人不在线)时保留输入,方便修改后重发。
                alert(err && err.error ? err.error : '发送失败');
                messageInput.focus();
                return;
            }
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
        makeDMTrigger(name, u.nick);
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

// makeDMTrigger 让在线用户名可点击:在输入框开头插入 @昵称,变成私聊。
function makeDMTrigger(el, nick) {
    const input = document.getElementById('chat-message');
    if (!input || nick === '(anonymous)') return;

    el.style.cursor = 'pointer';
    el.title = '私聊 ' + nick;
    el.classList.add('fw-semibold');
    el.addEventListener('click', () => {
        const prefix = '@' + nick + ' ';
        if (!input.value.startsWith(prefix)) {
            input.value = prefix + input.value;
        }
        input.focus();
    });
}

function newChatMessage(e) {
    const data = JSON.parse(e.data);
    const nick = data.nick;
    const message = data.message;
    const isPrivate = data.private === true;
    const style = isPrivate ? 'table-primary' : rowStyle(nick);
    const badge = isPrivate
        ? `<span class="badge text-bg-dark me-1" title="只有你和 @${escapeHtml(data.to)} 可见">私信</span>`
        : '';

    const tr = document.createElement('tr');
    tr.className = style;
    tr.innerHTML = `<td>${badge}${escapeHtml(nick)}</td><td>${escapeHtml(message)}</td>`;

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
