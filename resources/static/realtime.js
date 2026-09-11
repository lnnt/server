// 全局图表实例
let heapChart, mallocsChart, messagesChart;

function StartRealtime(roomid, timestamp) {
    StartCharts(timestamp);
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

function StartCharts(timestamp) {
    const windowSize = 60;
    const labels = [];
    const zeros = [];

    for (let i = 0; i < windowSize; i++) {
        labels.push(timestamp - windowSize + i);
        zeros.push(0);
    }

    // 公共配置
    const commonOptions = {
        responsive: true,
        maintainAspectRatio: false,
        animation: false,
        scales: {
            x: {
                display: true,
                ticks: { maxTicksLimit: 6 }
            },
            y: {
                beginAtZero: true
            }
        },
        plugins: {
            legend: { display: false }
        }
    };

    // Messages 图表
    messagesChart = new Chart(document.getElementById('messagesChart'), {
        type: 'line',
        data: {
            labels: [...labels],
            datasets: [
                { label: 'Users', data: [...zeros], borderColor: '#0d6efd', backgroundColor: 'rgba(13,110,253,0.1)', fill: true, tension: 0.3 },
                { label: 'Inbound', data: [...zeros], borderColor: '#fd7e14', backgroundColor: 'rgba(253,126,20,0.1)', fill: true, tension: 0.3 },
                { label: 'Outbound', data: [...zeros], borderColor: '#198754', backgroundColor: 'rgba(25,135,84,0.1)', fill: true, tension: 0.3 }
            ]
        },
        options: commonOptions
    });

    // Heap 图表
    heapChart = new Chart(document.getElementById('heapChart'), {
        type: 'line',
        data: {
            labels: [...labels],
            datasets: [
                { label: 'Heap', data: [...zeros], borderColor: '#0d6efd', backgroundColor: 'rgba(13,110,253,0.15)', fill: true, tension: 0.3 },
                { label: 'Stack', data: [...zeros], borderColor: '#6ea8fe', backgroundColor: 'rgba(110,168,254,0.15)', fill: true, tension: 0.3 }
            ]
        },
        options: commonOptions
    });

    // Mallocs 图表
    mallocsChart = new Chart(document.getElementById('mallocsChart'), {
        type: 'line',
        data: {
            labels: [...labels],
            datasets: [
                { label: 'Mallocs', data: [...zeros], borderColor: '#6610f2', backgroundColor: 'rgba(102,16,242,0.15)', fill: true, tension: 0.3 },
                { label: 'Frees', data: [...zeros], borderColor: '#6f42c1', backgroundColor: 'rgba(111,66,193,0.15)', fill: true, tension: 0.3 }
            ]
        },
        options: commonOptions
    });
}

function StartSSE(roomid) {
    if (!window.EventSource) {
        alert('EventSource is not supported in this browser');
        return;
    }
    const source = new EventSource('/stream/' + roomid);
    source.addEventListener('message', newChatMessage, false);
    source.addEventListener('stats', onStats, false);
}

function onStats(e) {
    const data = JSON.parse(e.data);
    const ts = data.timestamp;

    // 推入新数据并保持窗口大小
    pushChartData(messagesChart, ts, [data.Connected, data.Inbound, data.Outbound]);
    pushChartData(heapChart, ts, [data.HeapInuse, data.StackInuse]);
    pushChartData(mallocsChart, ts, [data.Mallocs, data.Frees]);
}

function pushChartData(chart, timestamp, values) {
    if (!chart) return;

    chart.data.labels.push(timestamp);
    chart.data.labels.shift();

    values.forEach((v, i) => {
        chart.data.datasets[i].data.push(v);
        chart.data.datasets[i].data.shift();
    });

    chart.update('none'); // 无动画更新，更流畅
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