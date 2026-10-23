import { ChatSession } from './chat-session.mjs';

const chat = document.getElementById('chat');
const input = document.getElementById('input');
const send = document.getElementById('send');
const chips = document.getElementById('chips');
const wake = document.getElementById('wake');
let brainReady = false;
const cancelButton = document.getElementById('cancel');

// Keep the app alive while this window is open.
let alive;
function connectAlive() { if (!alive) { try { alive = new EventSource('/api/alive'); } catch {} } }
connectAlive();

function greetingWord() {
  const h = new Date().getHours();
  if (h >= 5 && h < 11) return '早上好';
  if (h >= 11 && h < 13) return '中午好';
  if (h >= 13 && h < 18) return '下午好';
  return '晚上好';
}

// Keep 小离 (right side of the picture) clear of the UI: pick the background offset that pushes her
// right edge (~77% of the image) to the window's right edge, then size the subtitle column and
// input bar so they end before her shoulder (~40% of the image width).
const app = document.querySelector('.app');
function layout() {
  const W = innerWidth, H = innerHeight, s = Math.max(W / 1654, H / 951), iw = 1654 * s;
  if (app.classList.contains('avatar-ready')) {
    const root = document.documentElement.style;
    if (W < 760) { root.removeProperty('--column'); root.removeProperty('--composer'); }
    else { root.setProperty('--column', Math.min(560, W * .45 - 40) + 'px'); root.setProperty('--composer', Math.min(520, W * .45 - 40) + 'px'); }
    return;
  }
  const off = Math.min(iw - W, Math.max(0, 0.77 * iw - W));
  app.style.backgroundPosition = (-off) + 'px center';
  const root = document.documentElement.style;
  if (W < 760) { root.removeProperty('--column'); root.removeProperty('--composer'); return; }
  const safe = 0.40 * iw - off - 24;   // px from the left edge where her shoulder begins
  root.setProperty('--column', Math.round(Math.max(300, Math.min(560, safe - 40))) + 'px');
  root.setProperty('--composer', Math.round(Math.max(340, Math.min(520, safe - 40 - 84))) + 'px');
}
layout();
addEventListener('resize', layout);

function updateClock() {
  const now = new Date();
  document.getElementById('date').textContent = '今天 · 周' + '日一二三四五六'[now.getDay()] + ' · ' + (now.getMonth() + 1) + '月' + now.getDate() + '日';
  document.getElementById('greeting').textContent = greetingWord() + ' · ' + String(now.getHours()).padStart(2, '0') + ':' + String(now.getMinutes()).padStart(2, '0');
}
updateClock();
setInterval(updateClock, 30000);

function addBubble(who, text) {
  const d = document.createElement('div');
  d.className = 'msg ' + (who === 'me' ? 'me' : 'her');
  d.textContent = text;
  if (who === 'me') {
    for (const message of chat.children) message.classList.add('older');
  }
  chat.appendChild(d);
  chat.scrollTop = chat.scrollHeight;
  return d;
}

function typingBubble() {
  const d = addBubble('her', '');
  d.innerHTML = '<span class="dots"><span></span><span></span><span></span></span>';
  return d;
}

function setBusy(b) {
  send.disabled = b || !brainReady;
  cancelButton.classList.toggle('hidden', !b);
  input.disabled = false;
  if (!b) input.focus();
}

async function typeOut(el, text) {
  el.textContent = '';
  for (const ch of text) {
    el.textContent += ch;
    chat.scrollTop = chat.scrollHeight;
    await new Promise(r => setTimeout(r, 28));
  }
}

function showChips() {
  const ideas = ['帮我写一条请假消息', '这条短信是不是骗子？', '今晚吃点什么好？', '陪我随便聊聊'];
  chips.innerHTML = '';
  for (const s of ideas) {
    const b = document.createElement('button');
    b.className = 'chip'; b.textContent = s;
    b.onclick = () => { input.value = s; ask(); };
    chips.appendChild(b);
  }
  chips.classList.remove('hidden');
}

const bubbles = new Map();
let avatar = null;
let avatarFailed = false;
let latestMotion = 'idle';
const motionText = {idle:'安静陪伴', thinking:'正在思考', expressing:'正在回应'};
const avatarStatus = document.getElementById('avatar-status');
function setMotionState(state) {
  latestMotion = state;
  app.dataset.motionState = state;
  avatarStatus.textContent = avatarFailed ? '3D 暂不可用，仍可继续聊天' : '测试人偶 · ' + motionText[state];
  try { avatar?.setState(state); } catch { failAvatar(); }
}
function failAvatar() {
  avatarFailed = true;
  app.classList.remove('avatar-ready');
  document.getElementById('avatar-stage').hidden = true;
  app.dataset.avatar = 'unavailable';
  avatarStatus.textContent = '3D 暂不可用，仍可继续聊天';
  layout();
}
setMotionState('idle');
// This optional module is deliberately not a dependency of the chat controller.
import('./avatar.mjs').then(({createAvatar}) => {
  avatar = createAvatar(document.getElementById('avatar-stage'), {onFailure:failAvatar});
  if (avatarFailed) return;
  app.classList.add('avatar-ready');app.dataset.avatar = 'ready';
  avatar.setState(latestMotion);layout();
}).catch(failAvatar);

const session = new ChatSession({
  onState: setMotionState,
  onStart({id,text}) {
    chips.classList.add('hidden');input.value = '';autosize();
    addBubble('me',text);bubbles.set(id,typingBubble());setBusy(true);
  },
  onText({id,text}) {
    const bubble = bubbles.get(id);if (bubble) bubble.textContent = text;
    chat.scrollTop = chat.scrollHeight;
  },
  onFinish({id,status,reason,text}) {
    const bubble = bubbles.get(id);bubbles.delete(id);
    if (bubble) {
      if (status === 'completed') bubble.textContent = text;
      else if (status === 'cancelled') bubble.textContent = reason === 'hidden' ? '已暂停这次回复，回来后可以继续提问。' : '已停止这次回复，你可以继续说。';
      else bubble.textContent = reason === 'timeout' ? '这次回复等得有点久，请再试一次。' : '哎呀，我刚才走神了，没听清。你能再说一遍吗？';
    }
    setBusy(false);
  }
});
function ask() {
  if (!brainReady || session.busy) return;
  const text = input.value.trim();if (text) void session.send(text);
}
cancelButton.onclick = () => session.cancel('cancelled');
document.addEventListener('visibilitychange', () => { if (document.hidden) session.cancel('hidden'); });
addEventListener('pagehide', () => { session.cancel('hidden');alive?.close();alive = null; });
addEventListener('pageshow', () => { connectAlive();if (!session.busy) setMotionState('idle'); });

function autosize() { input.style.height = 'auto'; input.style.height = Math.min(input.scrollHeight, 120) + 'px'; document.documentElement.style.setProperty('--input-height', Math.max(52, input.offsetHeight + 12) + 'px'); }
input.addEventListener('input', autosize);
input.addEventListener('keydown', e => {
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing && e.keyCode !== 229) { e.preventDefault(); ask(); }
});
send.onclick = ask;

async function waitForBrain() {
  for (;;) {
    try {
      const s = await (await fetch('/api/status', { cache: 'no-store' })).json();
      if (s.ready) return true;
      if (s.failed) { wake.textContent = s.message || '小离没能醒来，请关掉窗口再试一次。'; return false; }
    } catch (e) {}
    await new Promise(r => setTimeout(r, 500));
  }
}

(async () => {
  const ok = await waitForBrain();
  if (!ok) return;
  wake.classList.add('hidden');
  const hello = greetingWord() + '呀！我是小离～\n有什么想问的、想聊的，直接在下面打字告诉我就好。不知道说什么的话，点点下面的小卡片也行。';
  const b = addBubble('her', '');
  await typeOut(b, hello);
  session.history.push({ role: 'assistant', content: hello });
  brainReady = true;
  showChips();
  input.disabled = false; send.disabled = false; input.focus();
})();
