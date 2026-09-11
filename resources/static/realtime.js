function StartRealtime(roomid, timestamp) {
    StartEpoch(timestamp);
    StartSSE(roomid);
    StartForm();
}

function StartForm() {
    const messageInput = document.getElementById('chat-message');
    const form = document.getElementById('chat-form');

    if (!messageInput || !form) return;

    messageInput.focus();

    form.addEventListener('submit', async function (e) {
        e.preventDefault();

        const formData = new FormData(form);

        try {
            await fetch(form.action, {
                method: 'POST',
                body: formData,
                // 不强制设置 Content-Type，让浏览器自动处理 multipart/form-data
            });
            messageInput.value = '';
            messageInput.focus();
        } catch (err) {
            console.error('Failed to send message:', err);
        }
    });
}

function StartEpoch(timestamp) {
    const windowSize = 60;
    const height = 200;
    const defaultData = histogram(windowSize, timestamp);

    // 注意：Epoch 仍通过 jQuery 风格初始化。若页面完全无 jQuery，
    // 需要额外引入一个极简 jQuery 兼容层或替换为现代图表库。
    // 这里保留原 Epoch 调用方式（假设页面仍能提供 $ 或你自行适配）。
    window.heapChart = $('#heapChart').epoch({
        type: 'time.area',
        axes: ['bottom', 'left'],
        height: height,
        historySize: 10,
        data: [
            { values: defaultData },
            { values: defaultData }
        ]
    });

    window.mallocsChart = $('#mallocsChart').epoch({
        type: 'time.area',
        axes: ['bottom', 'left'],
        height: height,
        historySize: 10,
        data: [
            { values: defaultData },
            { values: defaultData }
        ]
    });

    window.messagesChart = $('#messagesChart').epoch({
        type: 'time.line',
        axes: ['bottom', 'left'],
        height: 240,
        historySize: 10,
        data: [
            { values: defaultData },
            { values: defaultData },
            { values: defaultData }
        ]
    });
}

function StartSSE(roomid) {
    if (!window.EventSource) {
        alert('EventSource is not enabled in this browser');
        return;
    }
    const source = new EventSource('/stream/' + roomid);
    source.addEventListener('message', newChatMessage, false);
    source.addEventListener('stats', stats, false);
}

function stats(e) {
    const data = parseJSONStats(e.data);
    if (window.heapChart) heapChart.push(data.heap);
    if (window.mallocsChart) mallocsChart.push(data.mallocs);
    if (window.messagesChart) messagesChart.push(data.messages);
}

function parseJSONStats(raw) {
    const data = JSON.parse(raw);
    const timestamp = data.timestamp;

    const heap = [
        { time: timestamp, y: data.HeapInuse },
        { time: timestamp, y: data.StackInuse }
    ];

    const mallocs = [
        { time: timestamp, y: data.Mallocs },
        { time: timestamp, y: data.Frees }
    ];

    const messages = [
        { time: timestamp, y: data.Connected },
        { time: timestamp, y: data.Inbound },
        { time: timestamp, y: data.Outbound }
    ];

    return { heap, mallocs, messages };
}

function newChatMessage(e) {
    const data = JSON.parse(e.data);
    const nick = data.nick;
    const message = data.message;
    const style = rowStyle(nick);

    const html = `<tr class="${style}"><td>${escapeHtml(nick)}</td><td>${escapeHtml(message)}</td></tr>`;

    const chat = document.getElementById('chat');
    const scroll = document.getElementById('chat-scroll');

    if (chat) {
        chat.insertAdjacentHTML('beforeend', html);
    }
    if (scroll) {
        scroll.scrollTop = scroll.scrollHeight;
    }
}

function histogram(windowSize, timestamp) {
    const entries = new Array(windowSize);
    for (let i = 0; i < windowSize; i++) {
        entries[i] = { time: (timestamp - windowSize + i - 1), y: 0 };
    }
    return entries;
}

const entityMap = {
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
    '/': '&#x2F;'
};

function rowStyle(nick) {
    const classes = ['active', 'success', 'info', 'warning', 'danger'];
    const index = hashCode(nick) % 5;
    return classes[index];
}

function hashCode(s) {
    return Math.abs(
        s.split('').reduce(function (a, b) {
            a = ((a << 5) - a) + b.charCodeAt(0);
            return a & a;
        }, 0)
    );
}

function escapeHtml(string) {
    return String(string).replace(/[&<>"'\/]/g, function (s) {
        return entityMap[s];
    });
}

window.StartRealtime = StartRealtime;