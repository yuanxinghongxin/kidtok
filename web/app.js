// 原生版抖音式 Feed：全屏滑动 + 右侧互动栏 + 双击点赞 + 合集面板。
// 播放走同源代理 /api/video/stream（该接口内部会实时向网盘换取直链），
// 不用 raw_url 直连是因为浏览器会被 ORB 拦截。
const DEFAULT_DIR = '/启蒙';

// iOS「添加到主屏幕」独立模式下的兜底：个别系统版本会把 webview 的高度算少
// 「状态栏」那一截——整页紧贴屏幕上沿、底部却空出一条黑边，首页/我的栏就贴不到底。
// 判定同时满足才补偿：页面确实从屏幕 0 点开始（env(safe-area-inset-top) > 0，
// 正常情况下页面已在状态栏下方，这个值是 0），且布局高度明显短于屏幕高度。
// 补偿方式是把整页整体下移差值，页面底边正好落到屏幕底部。
function fixStandaloneViewport() {
  const standalone =
    window.matchMedia('(display-mode: standalone)').matches ||
    window.navigator.standalone === true;
  if (!standalone) return;

  const probe = document.createElement('div');
  probe.style.cssText =
    'position:fixed;top:0;left:0;width:0;height:0;padding-top:env(safe-area-inset-top);' +
    'visibility:hidden;pointer-events:none';
  document.body.appendChild(probe);
  const insetTop = probe.getBoundingClientRect().height;
  probe.remove();

  const gap = Math.round(window.screen.height) - document.documentElement.clientHeight;
  if (insetTop < 8 || gap <= 20 || gap > 140) return;

  document.documentElement.style.transform = 'translateY(' + gap + 'px)';
}

fixStandaloneViewport();

// 独立（PWA）模式下关掉双指缩放。iOS 从 Safari 10 起会忽略 viewport 里的
// user-scalable=no，只有拦下 Safari 私有的 gesture 事件 + 多指 touchmove 才真正有效；
// 单指触摸一律放过，不干扰刷视频和拖进度条。
function lockPinchZoom() {
  const standalone =
    window.matchMedia('(display-mode: standalone)').matches ||
    window.navigator.standalone === true;
  if (!standalone) return;

  ['gesturestart', 'gesturechange', 'gestureend'].forEach((type) => {
    document.addEventListener(type, (event) => event.preventDefault(), { passive: false });
  });

  document.addEventListener(
    'touchmove',
    (event) => {
      if (event.touches.length > 1) event.preventDefault();
    },
    { passive: false }
  );
}

lockPinchZoom();

// 全部图标用手写内联 SVG，不依赖任何外部字体图标库（避免 CDN 挂掉时图裂）
const ICONS = {
  heartOutline:
    '<svg class="ico" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linejoin="round" aria-hidden="true"><path d="M12 20.4 4.2 12.9a5.4 5.4 0 0 1 7.6-7.7l.2.2.2-.2a5.4 5.4 0 0 1 7.6 7.7z"/></svg>',
  heartFill:
    '<svg class="ico" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 20.4 4.2 12.9a5.4 5.4 0 0 1 7.6-7.7l.2.2.2-.2a5.4 5.4 0 0 1 7.6 7.7z"/></svg>',
  comment:
    '<svg class="ico" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 3.4c-5 0-9 3.5-9 7.8 0 2.4 1.3 4.6 3.4 6l-.3 3.1 3.2-1.5c.9.2 1.8.3 2.7.3 5 0 9-3.5 9-7.8s-4-7.9-9-7.9z"/><circle cx="8.2" cy="11.2" r="1.05" fill="#0b0b0d"/><circle cx="12" cy="11.2" r="1.05" fill="#0b0b0d"/><circle cx="15.8" cy="11.2" r="1.05" fill="#0b0b0d"/></svg>',
  star:
    '<svg class="ico" viewBox="0 0 24 24" fill="currentColor" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round" aria-hidden="true"><path d="m12 3.6 2.7 5.5 6.1.9-4.4 4.3 1 6-5.4-2.8-5.4 2.8 1-6-4.4-4.3 6.1-.9z"/></svg>',
  share:
    '<svg class="ico" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M13.4 4.2c0-1 1.2-1.5 1.9-.8l6.8 6.1a1.1 1.1 0 0 1 0 1.6l-6.8 6.1c-.7.7-1.9.2-1.9-.8v-3c-5.3 0-8 1.6-9.5 5.2-.6 1.4-2.7.8-2.5-.8.4-4.7 3.4-9 12-9z"/></svg>',
  plus:
    '<svg class="ico" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" aria-hidden="true"><path d="M12 6.6v10.8M6.6 12h10.8"/></svg>',
  folder:
    '<svg class="ico" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M3.6 7.2a2 2 0 0 1 2-2h3.6l1.7 1.9h7.5a2 2 0 0 1 2 2v7.1a2 2 0 0 1-2 2H5.6a2 2 0 0 1-2-2z"/></svg>',
  music:
    '<svg class="ico" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 3v10.55A4 4 0 1 0 14 17V7h4V3z"/></svg>',
  play:
    '<svg class="ico" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M8 5.6v12.8l11-6.4z"/></svg>',
  volumeOn:
    '<svg class="ico" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M2.5 9.1h3.2L9.7 5.8v12.4L5.7 15H2.5z"/><path d="M13.38 9.3a3.6 3.6 0 0 1 0 5.4M17.84 6.7a6.4 6.4 0 0 1 0 10.6" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"/></svg>',
  volumeMute:
    '<svg class="ico" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M2.5 9.1h3.2L9.7 5.8v12.4L5.7 15H2.5z"/><path d="m12.6 8.5 7.8 7m0-7-7.8 7" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round"/></svg>',
  eye:
    '<svg class="ico" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M2.4 12S5.7 6.2 12 6.2 21.6 12 21.6 12 18.3 17.8 12 17.8 2.4 12 2.4 12z"/><circle cx="12" cy="12" r="2.7"/></svg>',
  eyeOff:
    '<svg class="ico" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 3.8 21 20.2"/><path d="M10.6 6.3A9.7 9.7 0 0 1 12 6.2c4.9 0 8.4 3.1 9.6 5.8a12 12 0 0 1-2.7 3.6"/><path d="M6.2 8.1C4.4 9.5 3.2 11 2.4 12c1.2 2.7 4.7 5.8 9.6 5.8 1.5 0 2.9-.3 4.1-.9"/><path d="M10.1 10.2a2.7 2.7 0 0 0 3.8 3.8"/></svg>',
};

const params = new URLSearchParams(location.search);
// 视频目录只从服务器配置的 BasePath 读取；URL 上的 path 仅用于临时调试
let currentDir = params.get('path') || DEFAULT_DIR;

const feedEl = document.getElementById('feed');
const muteBtn = document.getElementById('mute-btn');
const heartLayer = document.getElementById('heart-layer');
const pageHome = document.getElementById('page-home');
const pageMe = document.getElementById('page-me');
const navEmpty = document.getElementById('nav-empty');
const meCount = document.getElementById('me-count');
const meWorks = document.getElementById('me-works');
const meCollections = document.getElementById('me-collections');
const meCollectionsCard = document.getElementById('me-collections-card');
const meTabs = document.querySelectorAll('.me-tab');
const pageCollections = document.getElementById('page-collections');
const collectionsList = document.getElementById('collections-list');
const collectionsBack = document.getElementById('collections-back');
const meCover = document.getElementById('me-cover');
const serverBtn = document.getElementById('server-btn');
const serverSub = document.getElementById('server-sub');
const settingsDrawer = document.getElementById('settings-drawer');
const drawerClose = document.getElementById('drawer-close');
const meSettingsBtn = document.getElementById('me-settings-btn');
const clearCacheBtn = document.getElementById('clear-cache-btn');
const aboutBtn = document.getElementById('about-btn');
const defaultMuteRow = document.getElementById('default-mute-row');
const defaultMuteSwitch = document.getElementById('default-mute-switch');
const defaultMuteSub = document.getElementById('default-mute-sub');
const pageConfig = document.getElementById('page-config');
const cfgForm = document.getElementById('cfg-form');
const cfgBack = document.getElementById('cfg-back');
const cfgProtocol = document.getElementById('cfg-protocol');
const cfgAddress = document.getElementById('cfg-address');
const cfgPort = document.getElementById('cfg-port');
const cfgUsername = document.getElementById('cfg-username');
const cfgPassword = document.getElementById('cfg-password');
const cfgVideoPath = document.getElementById('cfg-video-path');
const cfgEye = document.getElementById('cfg-eye');
const cfgSave = document.getElementById('cfg-save');
const cfgError = document.getElementById('cfg-error');
const toastEl = document.getElementById('toast');
const tabButtons = document.querySelectorAll('.tab');
const navTabs = document.querySelectorAll('.nav-tab');
const sheet = document.getElementById('collection-sheet');
const sheetTitle = document.getElementById('sheet-title');
const sheetSub = document.getElementById('sheet-sub');
const sheetList = document.getElementById('sheet-list');
const sheetClose = document.getElementById('sheet-close');
const backBtn = document.getElementById('back-to-feed');
const refreshBtn = document.getElementById('refresh-btn');
const pullEl = document.getElementById('pull-refresh');
const pullText = document.getElementById('pull-text');
const preloadSub = document.getElementById('preload-sub');
const preloadValueEl = document.getElementById('preload-value');
const preloadMinus = document.getElementById('preload-minus');
const preloadPlus = document.getElementById('preload-plus');

// 默认静音偏好：存在 localStorage，重开 App 仍生效。默认「否」＝不静音
const DEFAULT_MUTE_KEY = 'qimeng.defaultMuted';

function readDefaultMuted() {
  try {
    return localStorage.getItem(DEFAULT_MUTE_KEY) === '1';
  } catch (err) {
    return false;
  }
}

function saveDefaultMuted(value) {
  try {
    localStorage.setItem(DEFAULT_MUTE_KEY, value ? '1' : '0');
  } catch (err) {
    // 隐私模式等不可写时忽略：只是不持久化，本次运行仍然生效
  }
}

// 预加载数量偏好：当前屏之后同时准备几屏（默认 1，和以前的行为一致）。
// 数值越大切到下一屏越快，但并发请求更多、更费流量与网盘调用。
const PRELOAD_KEY = 'qimeng.preloadCount';
const PRELOAD_MIN = 0;
const PRELOAD_MAX = 5;

function readPreloadCount() {
  try {
    const raw = localStorage.getItem(PRELOAD_KEY);
    if (raw === null || raw === '') return 1;
    const n = Number(raw);
    if (!Number.isFinite(n)) return 1;
    return Math.min(PRELOAD_MAX, Math.max(PRELOAD_MIN, Math.round(n)));
  } catch (err) {
    return 1;
  }
}

function savePreloadCount(value) {
  try {
    localStorage.setItem(PRELOAD_KEY, String(value));
  } catch (err) {
    // 隐私模式等不可写时忽略：只是不持久化，本次运行仍然生效
  }
}

let preloadCount = readPreloadCount();

// 当前是否静音；启动时按用户设置决定（默认不静音）
let muted = readDefaultMuted();
let defaultMuted = muted;
let activeSlide = null;
// 预读白名单：当前屏 + 后面 preloadCount 屏持有 src，其余一律释放
let preloadSlides = new Set();
// 当前真正持有数据源（src 或 mpegts 播放器）的屏，用于统一做白名单清理
const loadedSlides = new Set();
// 快速滑动期间禁止一切预读，避免并发请求触发网盘风控
let fastScrolling = false;
let navMode = 'recommend';

// ---------- 数据源 ----------
// 合集列表（mock：当前目录当作一个合集，之后接真实接口时按合集分组即可）
const collections = [];
// 两份互相独立的数据：推荐流（所有合集的视频打乱）/ 当前合集（按集数正序）
let feedList = [];
let collectionList = [];

// ---------- 播放状态机 ----------
let currentMode = 'feed'; // 'feed' 推荐流 | 'collection' 合集连播
let currentPlayIndex = 0; // 当前播放的索引（相对当前数据源）
let returnIndex = 0; // 进入合集前，主 Feed 停留的位置
let activeCollection = null; // collection 模式下正在连播的合集
let dataSource = []; // 当前渲染到 DOM、正在滚动的数据源
let observer = null; // 当前数据源对应的 IntersectionObserver

function getCollection(id) {
  return collections.find((col) => col.id === id) || collections[0];
}

// Fisher-Yates 洗牌：推荐流每次进入都是新的随机顺序
function shuffle(list) {
  const arr = list.slice();
  for (let i = arr.length - 1; i > 0; i -= 1) {
    const j = Math.floor(Math.random() * (i + 1));
    [arr[i], arr[j]] = [arr[j], arr[i]];
  }
  return arr;
}

// 视频属于根目录下的哪个「一级子目录」（如 /启蒙/瑞奇宝宝、/启蒙/贝瓦儿歌）。
// 直接放在根目录里的视频单独归一组。
function topDirOf(path, root) {
  const base = root && root !== '/' ? root.replace(/\/+$/, '') + '/' : '/';
  const rel = path.startsWith(base) ? path.slice(base.length) : path.replace(/^\//, '');
  return rel.includes('/') ? rel.split('/')[0] : '（根目录视频）';
}

// 推荐流：按「根目录下的一级子目录」轮流排布，一轮 = 每个目录各出一条。
// 这样刷新后最前面的一批视频（数量≈目录总数）一定来自不同目录，
// 不会翻来覆去只刷到那几部，其余目录永远轮不到；每个目录内部的集数顺序随机，
// 所以同一部剧每次看的集也不同。
function interleaveByDirectory(list, root) {
  const buckets = new Map();
  list.forEach((item) => {
    const key = topDirOf(item.path, root);
    if (!buckets.has(key)) buckets.set(key, []);
    buckets.get(key).push(item);
  });

  const dirs = [...buckets.keys()];
  const queues = new Map(dirs.map((key) => [key, shuffle(buckets.get(key))]));

  const out = [];
  let lastDir = null;
  while (out.length < list.length) {
    // 每轮都重新随机目录顺序；只把还有视频的目录放进这一轮
    const round = shuffle(dirs.filter((key) => queues.get(key).length > 0));
    // 别让上一轮的最后一条和这一轮的第一条来自同一个目录
    if (round.length > 1 && round[0] === lastDir) {
      [round[0], round[1]] = [round[1], round[0]];
    }
    round.forEach((key) => {
      const item = queues.get(key).shift();
      if (!item) return;
      out.push(item);
      lastDir = key;
    });
  }
  return out;
}

// mock 数据：作者、文案，点赞等数字每次进入都随机
const AUTHORS = ['启蒙小课堂', '宝宝爱看', '小星星早教', '萌娃动画屋'];
const CAPTIONS = [
  '#启蒙 #早教 孩子最爱看的视频\n每天十分钟，越看越聪明',
  '#儿童动画 #认知 认识颜色和形状\n跟着一起读一遍吧',
  '#儿歌 #律动 边唱边跳更易记\n适合 2-6 岁小朋友',
];

async function api(path) {
  const res = await fetch(path);
  const body = await res.json();
  if (body.code !== 0) {
    throw new Error(body.msg || '请求失败');
  }
  return body.data;
}

function showEmpty(text) {
  feedEl.innerHTML = '';
  const tip = document.createElement('div');
  tip.className = 'empty';
  tip.textContent = text;
  feedEl.appendChild(tip);
}

// ---------- 冷启动占位 ----------
// 刷新页面后要先拿配置、再列目录，这段时间整屏是黑的（只有「加载中…」）。
// 打开 App 时先用「上次播放那一屏」的封面铺满屏，feed 回来后再换成真实列表。
const LAST_COVER_KEY = 'qimeng.lastCover';

function readLastCover() {
  try {
    const raw = localStorage.getItem(LAST_COVER_KEY);
    if (!raw) return null;
    const data = JSON.parse(raw);
    if (!data || typeof data.cover !== 'string' || !/^https?:/i.test(data.cover)) return null;
    return data;
  } catch (err) {
    return null;
  }
}

function saveLastCover(cover) {
  // 必须是真实图片地址：封面为空时 img.src 会变成当前页面地址，存下来也没用
  if (!cover || !/^https?:/i.test(cover)) return;
  try {
    localStorage.setItem(LAST_COVER_KEY, JSON.stringify({ cover }));
  } catch (err) {
    // 隐私模式等不可写时忽略：只是没有占位图
  }
}

function showBootPreview() {
  const last = readLastCover();
  if (!last) return;

  const wrap = document.createElement('div');
  wrap.className = 'boot-preview';

  const blurBg = document.createElement('img');
  blurBg.className = 'bg-blur';
  blurBg.alt = '';
  blurBg.src = last.cover;

  const cover = document.createElement('img');
  cover.className = 'cover';
  cover.alt = '';
  cover.src = last.cover;
  // 缩略图链接过期/失效时退回「加载中…」，而不是卡一张裂图
  cover.onerror = () => {
    wrap.remove();
    showEmpty('加载中…');
  };

  wrap.append(blurBg, cover);
  feedEl.replaceChildren(wrap);
}

function fmtTime(sec) {
  if (!isFinite(sec) || sec < 0) sec = 0;
  const m = Math.floor(sec / 60);
  const s = Math.floor(sec % 60);
  return m + ':' + String(s).padStart(2, '0');
}

function formatCount(n) {
  if (n >= 10000) {
    const v = Math.round(n / 1000) / 10;
    return (v % 1 === 0 ? v.toFixed(0) : v.toFixed(1)) + '万';
  }
  return String(n);
}

function mockFor(index) {
  return {
    author: AUTHORS[index % AUTHORS.length],
    caption: CAPTIONS[index % CAPTIONS.length],
    likes: Math.floor(1200 + Math.random() * 98000),
    comments: Math.floor(20 + Math.random() * 800),
    stars: Math.floor(10 + Math.random() * 600),
    shares: Math.floor(5 + Math.random() * 300),
  };
}

// ---------- 进度条（贴底线，整条区域可拖动） ----------
// 抖音的手感：按住任意位置就能拖、拖动中实时 seek、进度末端跟一个圆点，
// 上方气泡实时显示「当前时间 / 总时长」，松手就停在手指的位置。
function bindScrub(slide, video) {
  const scrub = document.createElement('div');
  scrub.className = 'scrub hide';

  const track = document.createElement('div');
  track.className = 'scrub-track';
  const fill = document.createElement('div');
  fill.className = 'scrub-fill';
  const thumb = document.createElement('span');
  thumb.className = 'scrub-thumb';
  fill.appendChild(thumb);
  track.appendChild(fill);

  const bubble = document.createElement('div');
  bubble.className = 'scrub-bubble';
  bubble.textContent = '0:00 / 0:00';

  scrub.appendChild(track);
  scrub.appendChild(bubble);
  slide.appendChild(scrub);

  let dragging = false;

  const ratioAt = (clientX) => {
    const rect = track.getBoundingClientRect();
    if (rect.width <= 0) return 0;
    return Math.min(1, Math.max(0, (clientX - rect.left) / rect.width));
  };

  const durationOf = () => {
    const d = video.duration;
    return isFinite(d) && d > 0 ? d : 0;
  };

  const paint = (ratio) => {
    fill.style.width = (ratio * 100).toFixed(2) + '%';
    // 气泡跟着手指走，贴边时收一点，避免半个气泡超出屏幕
    bubble.style.left = Math.min(92, Math.max(8, ratio * 100)) + '%';
    const total = durationOf();
    bubble.textContent = fmtTime(ratio * total) + ' / ' + fmtTime(total);
  };

  // 拖动中实时改 currentTime，画面跟着手指走；不暂停播放，
  // 这样不会弹出屏幕中间的大播放图标
  const seek = (ratio) => {
    const total = durationOf();
    if (total > 0) video.currentTime = ratio * total;
  };

  function refresh() {
    if (dragging) return;
    const duration = video.duration;
    if (!isFinite(duration) || duration <= 0) {
      scrub.classList.add('hide');
      return;
    }
    scrub.classList.remove('hide');
    fill.style.width = ((video.currentTime / duration) * 100).toFixed(2) + '%';
    bubble.textContent = fmtTime(video.currentTime) + ' / ' + fmtTime(duration);
  }

  const begin = (clientX) => {
    if (durationOf() === 0) return false;
    dragging = true;
    scrub.classList.add('dragging');
    const ratio = ratioAt(clientX);
    paint(ratio);
    seek(ratio);
    return true;
  };

  const move = (clientX) => {
    if (!dragging) return;
    const ratio = ratioAt(clientX);
    paint(ratio);
    seek(ratio);
  };

  const end = (clientX) => {
    if (!dragging) return;
    if (clientX != null) move(clientX);
    dragging = false;
    scrub.classList.remove('dragging');
    refresh();
  };

  const cancel = () => {
    if (!dragging) return;
    dragging = false;
    scrub.classList.remove('dragging');
    refresh();
  };

  // 触摸必须走 touch 事件并在 touchstart 里 preventDefault：
  // 只靠 pointer 事件时，iOS 会把手指的拖动识别成列表滚动并触发 pointercancel，
  // 拖动就被系统打断（这就是之前「拖不动」的原因）
  scrub.addEventListener(
    'touchstart',
    (event) => {
      if (!event.touches.length) return;
      if (begin(event.touches[0].clientX)) event.preventDefault();
    },
    { passive: false }
  );

  scrub.addEventListener(
    'touchmove',
    (event) => {
      if (!dragging || !event.touches.length) return;
      event.preventDefault();
      move(event.touches[0].clientX);
    },
    { passive: false }
  );

  scrub.addEventListener(
    'touchend',
    (event) => {
      if (!dragging) return;
      const touch = event.changedTouches && event.changedTouches[0];
      event.preventDefault();
      end(touch ? touch.clientX : null);
    },
    { passive: false }
  );

  scrub.addEventListener('touchcancel', cancel);

  // 鼠标 / 触控笔：按下后把指针捕获在进度条上，拖出条外也继续跟手
  scrub.addEventListener('pointerdown', (event) => {
    if (event.pointerType === 'touch') return; // 触摸已由上面的 touch 事件处理
    if (!begin(event.clientX)) return;
    try {
      scrub.setPointerCapture(event.pointerId);
    } catch (err) {
      // 个别环境不支持指针捕获，忽略即可（仍能拖动，只是拖出条外会断）
    }
    event.preventDefault();
    event.stopPropagation();
  });

  scrub.addEventListener('pointermove', (event) => {
    if (event.pointerType === 'touch') return;
    move(event.clientX);
  });

  scrub.addEventListener('pointerup', (event) => {
    if (event.pointerType === 'touch') return;
    end(event.clientX);
  });

  scrub.addEventListener('pointercancel', (event) => {
    if (event.pointerType === 'touch') return;
    cancel();
  });

  video.addEventListener('timeupdate', refresh);
  video.addEventListener('loadedmetadata', refresh);
  video.addEventListener('durationchange', refresh);
}

// ---------- 右侧互动栏 ----------
function buildRail(slide) {
  const rail = document.createElement('div');
  rail.className = 'rail';

  const avatarWrap = document.createElement('button');
  avatarWrap.type = 'button';
  avatarWrap.className = 'avatar-wrap';
  avatarWrap.title = '查看合集';
  const avatar = document.createElement('span');
  avatar.className = 'avatar';
  avatar.textContent = '启';
  const plus = document.createElement('span');
  plus.className = 'avatar-plus';
  plus.innerHTML = ICONS.plus;
  avatarWrap.appendChild(avatar);
  avatarWrap.appendChild(plus);
  avatarWrap.addEventListener('click', () => openSheet(slide));
  rail.appendChild(avatarWrap);

  [
    { key: 'likes', icon: ICONS.heartOutline, cls: 'act-like' },
    { key: 'comments', icon: ICONS.comment, cls: '' },
    { key: 'stars', icon: ICONS.star, cls: '' },
    { key: 'shares', icon: ICONS.share, cls: '' },
  ].forEach((item) => {
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = ('act ' + item.cls).trim();
    btn.dataset.count = item.key;
    const icon = document.createElement('span');
    icon.className = 'act-icon';
    icon.innerHTML = item.icon;
    const label = document.createElement('span');
    label.className = 'act-count';
    btn.appendChild(icon);
    btn.appendChild(label);

    btn.addEventListener('click', () => {
      if (item.key === 'likes') {
        const box = btn.getBoundingClientRect();
        likeSlide(slide, box.left + box.width / 2, box.top + box.height / 2);
        return;
      }
      bump(btn);
    });
    rail.appendChild(btn);
  });

  const vinyl = document.createElement('div');
  vinyl.className = 'vinyl';
  vinyl.innerHTML = '<span class="vinyl-disc"><span class="vinyl-center">启</span></span>';
  rail.appendChild(vinyl);

  return rail;
}

// ---------- 左下信息区 ----------
function buildInfo(slide) {
  const info = document.createElement('div');
  info.className = 'info';

  const tag = document.createElement('button');
  tag.type = 'button';
  tag.className = 'collection-tag';
  tag.innerHTML = ICONS.folder + '<span class="collection-tag-text"></span>';
  tag.addEventListener('click', () => openSheet(slide));
  info.appendChild(tag);

  const author = document.createElement('h3');
  author.className = 'author';
  info.appendChild(author);

  const caption = document.createElement('p');
  caption.className = 'caption';
  info.appendChild(caption);

  const music = document.createElement('div');
  music.className = 'music';
  music.innerHTML =
    ICONS.music +
    '<div class="marquee"><div class="marquee-inner"><span></span></div></div>';
  info.appendChild(music);

  return info;
}

// 把某一集的信息（所属合集、第几集、作者、文案、各项计数）写进这一屏
function applyInfo(slide, col, episodeIndex) {
  const mock = mockFor(episodeIndex);
  slide._episodeIndex = episodeIndex;
  slide._collectionId = col.id;
  slide._mock = mock;
  slide._liked = false;

  const tagText = slide.querySelector('.collection-tag-text');
  if (!tagText) return;

  // 胶囊文案要能一眼看出「当前是哪一季的第几集」：
  // 分季合集写成「小皮大世界 · 第2季 · 第5集」，未分季的保持原来的写法
  const ep = col.episodes[episodeIndex] || {};
  let text = col.name;
  if (ep.season) text += ' · ' + ep.season;
  text += ' · 第' + (ep.season ? ep.seasonIndex + 1 : episodeIndex + 1) + '集';
  if (!ep.season) text += ' / 共 ' + col.episodes.length + ' 集';
  tagText.textContent = text;

  const avatar = slide.querySelector('.avatar');
  if (avatar) avatar.textContent = mock.author.slice(0, 1);
  // 黑胶唱片中间的「头像」也用同一个首字占位
  const vinylCenter = slide.querySelector('.vinyl-center');
  if (vinylCenter) vinylCenter.textContent = mock.author.slice(0, 1);

  slide.querySelector('.author').textContent = '@' + mock.author;
  slide.querySelector('.caption').textContent = mock.caption;

  const music = '@' + mock.author + ' 创作的原声';
  slide.querySelectorAll('.marquee-inner span').forEach((span) => {
    span.textContent = music;
  });

  slide.querySelectorAll('.act').forEach((btn) => {
    const count = btn.querySelector('.act-count');
    if (count) count.textContent = formatCount(mock[btn.dataset.count]);
  });

  const like = slide.querySelector('.act-like');
  if (like) {
    like.classList.remove('is-liked');
    const wrap = like.querySelector('.act-icon');
    if (wrap) wrap.innerHTML = ICONS.heartOutline;
  }
}

function buildSlide(item, index) {
  const col = getCollection(item.collectionId);
  const slide = document.createElement('section');
  slide.className = 'slide';
  slide.dataset.path = item.path;
  // 封面字段：feed 里的条目用的是 cover（由 API 的 cover_url 映射而来），
  // 这里兼容两种写法，取到真正的缩略图地址
  const coverUrl = item.cover || item.cover_url || '';
  slide._index = index; // 在当前数据源里的位置，供状态机同步 currentPlayIndex

  // 底层：同一张封面放大后高斯模糊 + 压暗，用来填补 16:9 视频左右的黑边
  const blurBg = document.createElement('img');
  blurBg.className = 'bg-blur';
  blurBg.alt = '';
  blurBg.loading = index < 2 ? 'eager' : 'lazy';
  // 没有封面时不要赋空串：空 src 会被解析成当前页面地址，加载失败后
  // 左上角会冒出一个小裂图图标（看起来像个白色小方块）
  if (coverUrl) blurBg.src = coverUrl;
  slide.appendChild(blurBg);

  const cover = document.createElement('img');
  cover.className = 'cover';
  cover.alt = '';
  cover.loading = index < 2 ? 'eager' : 'lazy';
  if (coverUrl) cover.src = coverUrl;
  slide.appendChild(cover);

  const video = document.createElement('video');
  video.className = 'player';
  video.muted = muted;
  video.loop = true;
  video.playsInline = true;
  video.preload = 'none';
  video.setAttribute('playsinline', '');
  // 起播前用封面当 poster：iOS 的 <video> 在没有首帧时会画一块白色底，
  // 表现为播放刚开始时画面外面闪一圈白边/白框，有 poster 就不会出现
  if (coverUrl) video.poster = coverUrl;

  const hint = document.createElement('div');
  hint.className = 'hint';

  video.addEventListener('loadeddata', () => {
    stopWatchdog(slide);
    slide.classList.add('playing');
  });

  // 封面淡出的时机：等「首帧真的画出来」再淡出（painted），而不是 loadeddata。
  // 因为 MSE(TS) 在 loadeddata 时可能还在解码，提前撤掉封面会露出一两秒黑屏；
  // 视频没出画面前一直显示高清封面，用户不会看到黑屏。
  const markPainted = () => {
    slide.classList.add('painted');
    hint.classList.remove('show');
    // 记住这一屏的封面，下次冷启动先用它铺满整屏，避免黑屏。
    // 取原始 src 属性：封面为空时 src 属性是 ''，而 img.src 会变成本页地址
    const coverImg = slide.querySelector('img.cover');
    if (coverImg) saveLastCover(coverImg.getAttribute('src'));
  };
  video.addEventListener('playing', markPainted);
  // 暂停态只由视频自身的 play/pause 事件驱动，保证图标和真实播放状态永远一致
  video.addEventListener('pause', () => slide.classList.add('paused'));
  video.addEventListener('play', () => slide.classList.remove('paused'));
  // 原生解码失败时再兜底试一次 mpegts（只在按容器判断为 mp4/unknown 时才会走到原生）
  video.addEventListener('error', () => {
    // 切集时会先清空 src，这里跳过空源触发的错误
    if (!video._url) return;
    // HLS 的错误交给 hls.js 实例自己处理，不要再走 mpegts 兜底
    if (video._hls) return;
    // 编码探测已经判定解不了（mp4v / HEVC），别再浪费时间走 mpegts 兜底
    if (slide._codecBad) {
      skipUnplayable(slide, slide._codec, false);
      return;
    }
    if (video._fallbackTried) {
      slide.classList.remove('playing');
      hint.textContent = '播放失败：该视频编码不支持';
      hint.classList.add('show');
      skipUnplayable(slide, slide._codec, !slide._codec);
      return;
    }
    video._fallbackTried = true;
    startMpegts(video, slide, slide.dataset.active === '1');
  });
  // 循环：原生播放靠 loop 属性；mpegts 的 MediaSource 到片尾已 endOfStream，只能重建播放器。
  // HLS 由 hls.js 自己按 loop 续播，这里不重建，避免把 HLS 实例误建成 mpegts 播放器。
  video.addEventListener('ended', () => {
    if (!video._player || video._hls) return;
    video._player.destroy();
    video._player = null;
    startMpegts(video, slide, slide.dataset.active === '1');
  });
  slide.appendChild(video);
  slide.appendChild(hint);

  // 暂停时落在屏幕正中的大号播放三角：不拦截点击，点它照样等同于点视频
  const pauseIcon = document.createElement('div');
  pauseIcon.className = 'pause-icon';
  pauseIcon.innerHTML =
    '<svg viewBox="0 0 24 24" aria-hidden="true">' +
    '<path d="M8.6 5.1a1 1 0 0 1 1.5-.86l10 6.9a1 1 0 0 1 0 1.72l-10 6.9a1 1 0 0 1-1.5-.86z"/>' +
    '</svg>';
  slide.appendChild(pauseIcon);

  const overlay = document.createElement('div');
  overlay.className = 'overlay';
  overlay.appendChild(buildRail(slide));
  overlay.appendChild(buildInfo(slide));
  slide.appendChild(overlay);

  bindScrub(slide, video);
  applyInfo(slide, col, item.episodeIndex);
  return slide;
}

// 每次切换模式都整段重建 DOM：旧的播放器/监听全部销毁，逻辑最简单也最不容易残留状态。
function teardownSlides() {
  if (observer) {
    observer.disconnect();
    observer = null;
  }
  activeSlide = null;
  preloadSlides.clear();
  fastScrolling = false;
  loadedSlides.clear();
  if (scrollEndTimer) {
    clearTimeout(scrollEndTimer);
    scrollEndTimer = null;
  }
  if (scrollWindowTimer) {
    clearTimeout(scrollWindowTimer);
    scrollWindowTimer = null;
  }
  feedEl.querySelectorAll('video').forEach((video) => {
    destroyHls(video);
    if (video._player) {
      try {
        video._player.destroy();
      } catch (err) {
        // 播放器已销毁时忽略
      }
      video._player = null;
    }
    video._url = '';
    video.removeAttribute('src');
    try {
      video.load();
    } catch (err) {
      // 忽略空源导致的异常
    }
  });
  feedEl.innerHTML = '';
}

function renderList(list, mode) {
  teardownSlides();
  dataSource = list;
  currentMode = mode;
  meCount.textContent = String(list.length);
  meWorks.textContent = String(list.length);

  // 没有占位封面时（比如首次安装），先用列表第一屏的封面兜底，
  // 这样下次冷启动至少有图可显示
  if (list.length && !readLastCover()) saveLastCover(list[0].cover);

  const fragment = document.createDocumentFragment();
  list.forEach((item, index) => fragment.appendChild(buildSlide(item, index)));
  feedEl.appendChild(fragment);
  observeSlides();
}

// 瞬间跳到某一屏：不用平滑滚动，避免重建后还要等动画结束。
function jumpToIndex(index) {
  if (index < 0 || index >= dataSource.length) return;
  const target = feedEl.querySelectorAll('.slide')[index];
  if (!target) return;
  feedEl.scrollTop = target.offsetTop;
  currentPlayIndex = index;

  // 跳转时旧屏立刻释放，保证同一时刻不会有 3 条连接
  const prev = activeSlide;
  if (prev && prev !== target) {
    prev.dataset.active = '';
    const prevVideo = prev.querySelector('video');
    if (prevVideo) pauseVideo(prevVideo);
    releaseSlide(prev);
    preloadSlides.delete(prev);
  }
  activate(target);
}

function observeSlides() {
  observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (entry.isIntersecting && entry.intersectionRatio >= 0.6) {
          activate(entry.target);
        } else {
          deactivate(entry.target);
        }
      }
    },
    { root: feedEl, threshold: [0, 0.6, 1] }
  );
  feedEl.querySelectorAll('.slide').forEach((slide) => observer.observe(slide));
}

// 自动播放被浏览器拦下时：先临时静音起播，声音留到用户第一次触摸再恢复。
// 不能在起播后立刻取消静音：iOS / 部分 WebView 在非用户手势里给视频
// 取消静音会直接把视频暂停，表现就是「每一屏都停在暂停状态」。
let pendingUnmute = false; // 有视频是靠临时静音才起播的
let pendingGesturePlay = false; // 连静音自动播放都被拦，等首次触摸再放

function startVideo(video) {
  return video._player ? video._player.play() : video.play();
}

// 立刻发起播放（同步，不要丢到微任务里）：手机上「用户手势」只在这一拍有效，
// 推迟执行 play() 就可能被浏览器当成非手势调用而拒绝。
function tryStartVideo(video) {
  try {
    return Promise.resolve(startVideo(video));
  } catch (err) {
    return Promise.reject(err);
  }
}

function playVideo(video) {
  // 还没定好播放方式（既没有 mpegts 播放器也没设 src）时直接跳过，交给 prepareSlide 后续再播
  if (!video._player && !video.src) return;
  video._userPaused = false;

  tryStartVideo(video).catch(() => {
    // 有声自动播放被拦：先静音让它动起来，标记一下，等手势再还声音
    video.muted = true;
    video._policyMuted = true;
    pendingUnmute = true;
    tryStartVideo(video).catch(() => {
      // 极少数 WebView 连静音自动播放都拦：保持暂停，等用户点一下再播
      video.muted = muted;
      pendingGesturePlay = true;
    });
  });
}

// 退回静音继续播：声音策略失败后的兜底，保证画面不会停在暂停态
function revertToMuted(video) {
  if (video._policyMuted) return;
  video._policyMuted = true;
  video.muted = true;
  tryStartVideo(video).catch(() => {});
}

// 刚取消静音时复查一次：有的 WebView 不报错，而是直接把这个视频暂停。
// 只要不是用户自己点的暂停，就退回静音继续播。
function guardUnmute(video) {
  tryStartVideo(video).catch(() => revertToMuted(video));
  setTimeout(() => {
    const slide = video.parentNode;
    if (slide && slide.dataset.active !== '1') return;
    if (!video._policyMuted && video.paused && !video._userPaused) revertToMuted(video);
  }, 400);
}

// 给某一屏设置声音；刚从「策略静音」恢复、且本来正在播放时要复查是否被系统暂停。
// 已经在暂停的屏（预读/滑走的屏）只改 muted，不主动起播，避免后台偷偷出声。
function applyMute(video, next) {
  const wasPolicyMuted = !!video._policyMuted;
  const wasPlaying = !video.paused;
  video._policyMuted = false;
  video.muted = next;
  if (!next && wasPolicyMuted && wasPlaying) guardUnmute(video);
}

// 用户第一次触摸/按键时：把「策略静音」的视频恢复成用户设置的声音；
// 若之前完全没起播，则借这次手势再播一次。
// 用 touchend / click：iOS 只认这两级用户手势，touchstart 不算。
function onFirstGesture() {
  if (pendingUnmute) {
    pendingUnmute = false;
    feedEl.querySelectorAll('video').forEach((video) => {
      if (video._policyMuted) applyMute(video, muted);
    });
  }
  if (pendingGesturePlay && activeSlide) {
    pendingGesturePlay = false;
    const video = activeSlide.querySelector('video');
    if (video && video.paused) playVideo(video);
  }
}

['touchend', 'click', 'keydown'].forEach((type) =>
  document.addEventListener(type, onFirstGesture, { capture: true })
);

function pauseVideo(video) {
  if (video._player) {
    video._player.pause();
  } else if (!video.paused) {
    video.pause();
  }
}

function setMuted(next) {
  muted = next;
  muteBtn.classList.toggle('is-muted', muted);
  muteBtn.innerHTML = muted ? ICONS.volumeMute : ICONS.volumeOn;
  feedEl.querySelectorAll('video').forEach((video) => {
    // 主动开声音时同样要复查：个别 WebView 会因取消静音把视频暂停
    applyMute(video, muted);
  });
}

// 设置页「是否默认静音」：写偏好 + 同步开关显示 + 立刻作用到当前视频
function applyDefaultMuted(value) {
  defaultMuted = value;
  saveDefaultMuted(value);
  defaultMuteSwitch.classList.toggle('on', value);
  defaultMuteRow.setAttribute('aria-pressed', value ? 'true' : 'false');
  defaultMuteSub.textContent = value ? '开：每次打开先静音' : '关：打开即有声';
  setMuted(value);
}

// 高级设置「预加载数量」：写偏好 + 同步显示 + 立刻按新数量补齐或收回预读
function applyPreloadCount(value) {
  const next = Math.min(PRELOAD_MAX, Math.max(PRELOAD_MIN, Math.round(value)));
  preloadCount = next;
  savePreloadCount(next);

  preloadValueEl.textContent = String(next);
  preloadSub.textContent = next === 0 ? '不预读：只加载当前屏' : '同时预读下面 ' + next + ' 屏';
  preloadMinus.disabled = next <= PRELOAD_MIN;
  preloadPlus.disabled = next >= PRELOAD_MAX;

  if (activeSlide) preloadNext();
}

// HLS.js 播放 m3u8：分片地址已由后端 /api/proxy 改写，浏览器只访问同源地址。
// 切屏时必须 destroy，否则每个滑过的视频都会留着一条拉流连接和 MediaSource。
function startHls(video, slide, autoplay) {
  const url = video._url;
  const hint = slide.querySelector('.hint');
  if (!url) return;

  // Safari 原生支持 HLS，直接赋 src 少一层 MSE，兼容性也更稳
  if (typeof Hls === 'undefined' || !Hls.isSupported()) {
    if (video.canPlayType('application/vnd.apple.mpegurl')) {
      video.src = url;
      video.loop = true;
      if (autoplay) playVideo(video);
      return;
    }
    hint.textContent = '当前浏览器不支持 HLS 播放';
    hint.classList.add('show');
    return;
  }

  try {
    const hls = new Hls({ enableWorker: true, backBufferLength: 30 });
    hls.on(Hls.Events.ERROR, (event, data) => {
      if (!data || !data.fatal) return;
      // 网络抖动/解码错误先尝试原地恢复，恢复不了才提示
      if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
        hls.startLoad();
        return;
      }
      if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
        hls.recoverMediaError();
        return;
      }
      hint.textContent = '播放失败：' + data.details;
      hint.classList.add('show');
    });
    hls.on(Hls.Events.MANIFEST_PARSED, () => {
      if (autoplay && slide.dataset.active === '1') playVideo(video);
    });
    hls.loadSource(url);
    hls.attachMedia(video);
    video._hls = hls;
    video.loop = true;
  } catch (err) {
    hint.textContent = 'HLS 初始化失败：' + err.message;
    hint.classList.add('show');
  }
}

// 销毁 HLS 实例。hls.js 的 destroy 会移除 MediaSource、断开监听并停止拉流。
function destroyHls(video) {
  if (!video._hls) return;
  try {
    video._hls.destroy();
  } catch (err) {
    // 已经销毁过时忽略
  }
  video._hls = null;
}

// mpegts.js 通过 MSE 播放 TS 流（片源后缀是 .mp4，实际容器是 MPEG-TS + H.264/AAC，
// Chrome 原生不支持 TS 容器）。依赖同源代理支持 Range，所以 seekType 用 'range'。
function startMpegts(video, slide, autoplay) {
  const url = video._url;
  const hint = slide.querySelector('.hint');
  if (!url || typeof mpegts === 'undefined' || !mpegts.isSupported()) {
    hint.textContent = '当前浏览器不支持该视频格式';
    hint.classList.add('show');
    return;
  }

  try {
    const player = mpegts.createPlayer(
      { type: 'mse', url: url, isLive: false },
      { enableWorker: false, seekType: 'range', lazyLoad: false }
    );
    player.on(mpegts.Events.ERROR, (type, detail) => {
      hint.textContent = '播放失败：' + type + '/' + detail;
      hint.classList.add('show');
    });
    player.attachMediaElement(video);
    player.load();
    video._player = player;
    video.loop = false;
    if (autoplay) playVideo(video);
  } catch (err) {
    hint.textContent = '解码失败：' + err.message;
    hint.classList.add('show');
  }
}

// 读取响应的第一块数据，读完立刻取消，避免上游忽略 Range 时把整个文件拉下来。
function readFirstChunk(res) {
  if (res.body && res.body.getReader) {
    const reader = res.body.getReader();
    return reader.read().then(({ value }) => {
      reader.cancel().catch(() => {});
      return value || new Uint8Array(0);
    });
  }
  return res.arrayBuffer().then((buf) => new Uint8Array(buf));
}

// 看开头几个字节判断容器：真 MP4 交给浏览器原生解码，MPEG-TS（后缀也可能叫 .mp4）交给 mpegts.js，
// 以 #EXTM3U 开头的播放列表交给 HLS.js。
function containerOf(bytes) {
  if (!bytes || bytes.length < 8) return 'unknown';
  if (bytes[4] === 0x66 && bytes[5] === 0x74 && bytes[6] === 0x79 && bytes[7] === 0x70) return 'mp4';
  if (bytes[0] === 0x23) {
    const head = new TextDecoder().decode(bytes.subarray(0, Math.min(bytes.length, 16)));
    if (head.indexOf('#EXTM3U') === 0) return 'hls';
  }
  if (bytes[0] === 0x47) return 'ts';
  if (bytes.length > 188 && bytes[188] === 0x47) return 'ts';
  return 'unknown';
}

async function sniffContainer(url) {
  try {
    const res = await fetch(url, { headers: { Range: 'bytes=0-2047' } });
    return containerOf(await readFirstChunk(res));
  } catch (err) {
    return 'unknown';
  }
}

// 首屏预热：feed 数据一到就把第一条视频的容器探测先发出去
// （这一下同时会让服务端把网盘直链取好，那一步要 1 秒左右），
// 与后面几百个 slide 的 DOM 构建并行，首屏起播能早一点。
const warmContainers = new Map();

function warmContainer(path) {
  if (!path) return Promise.resolve('unknown');
  const exist = warmContainers.get(path);
  if (exist) return exist;
  const url = '/api/video/stream?path=' + encodeURIComponent(path);
  const task = sniffContainer(url);
  warmContainers.set(path, task);
  return task;
}

// ---------- 编码探测 ----------
// 有些片源 Chrome 能解析 MP4 的 moov（时长、进度条都正常），却一帧都解不出来：
// MPEG-4 Part 2（mp4v）、HEVC（hev1/hvc1）都属于这种情况。表现就是画面全黑、
// 进度条照走、「加载中…」永远不消失。这里直接读 moov→stsd 拿到视频采样格式，
// 再问浏览器能不能解，解不了就跳过，别让用户干等。
const CODEC_FAMILY = {
  avc1: 'avc1.42E01E',
  av01: 'av01.0.05M.08',
  vp09: 'vp09.00.10.08',
  hev1: 'hev1.1.6.L120.90',
  hvc1: 'hvc1.1.6.L120.90',
  mp4v: 'mp4v.20.9',
};
const CODEC_LABEL = {
  mp4v: 'MPEG-4 Part 2',
  hev1: 'HEVC（H.265）',
  hvc1: 'HEVC（H.265）',
  av01: 'AV1',
  vp09: 'VP9',
};
// 音频采样格式，扫描 stsd 时跳过，避免拿到只含音轨的轨
const AUDIO_FOURCC = new Set(['mp4a', 'ac-3', 'ec-3', 'opus', 'alac', 'sowt', 'twos', 'spex']);
const codecCache = new Map();

// 从数据块里找 stsd 盒子并读出第一个采样格式：
// [size:4][stsd:4][version/flags:4][entry_count:4][entry_size:4][format:4]
function stsdFourcc(bytes) {
  for (let i = 0; i + 20 <= bytes.length; i += 1) {
    if (
      bytes[i] === 0x73 &&
      bytes[i + 1] === 0x74 &&
      bytes[i + 2] === 0x73 &&
      bytes[i + 3] === 0x64
    ) {
      const four = String.fromCharCode(
        bytes[i + 16],
        bytes[i + 17],
        bytes[i + 18],
        bytes[i + 19]
      );
      if (/^[0-9a-z]{4}$/i.test(four) && !AUDIO_FOURCC.has(four)) return four;
    }
  }
  return null;
}

// 读够 max 字节后立刻取消，避免上游忽略 Range 时把整个文件拉下来
async function readBytes(res, max) {
  if (!res.body || !res.body.getReader) {
    const buf = await res.arrayBuffer();
    return new Uint8Array(buf.slice(0, max));
  }
  const reader = res.body.getReader();
  let out = new Uint8Array(0);
  try {
    while (out.length < max) {
      const { value, done } = await reader.read();
      if (done || !value) break;
      const merged = new Uint8Array(out.length + value.length);
      merged.set(out);
      merged.set(value, out.length);
      out = merged;
    }
  } finally {
    reader.cancel().catch(() => {});
  }
  return out.slice(0, max);
}

// moov 可能在文件头（faststart）也可能在文件尾，两头各取 256KB 找 stsd
// （实测 stsd 距文件尾 90~200KB，距文件头同理，所以 256KB 足够覆盖）
const PROBE_BYTES = 262144;

async function probeCodec(url) {
  if (!url) return null;
  if (codecCache.has(url)) return codecCache.get(url);

  const task = (async () => {
    try {
      const headRange = 'bytes=0-' + (PROBE_BYTES - 1);
      let four = stsdFourcc(await readBytes(await fetch(url, { headers: { Range: headRange } }), PROBE_BYTES));
      if (four) return four;
      const tailRange = 'bytes=-' + PROBE_BYTES;
      four = stsdFourcc(await readBytes(await fetch(url, { headers: { Range: tailRange } }), PROBE_BYTES));
      return four;
    } catch (err) {
      return null;
    }
  })();
  codecCache.set(url, task);
  return task;
}

// 探测不到编码时不拦（交给播放器 + 卡死看门狗兜底）
function codecPlayable(four) {
  if (!four) return true;
  // MPEG-4 Part 2（老式 DivX/Xvid）浏览器一律解不了，不用问能力检测
  if (four === 'mp4v') return false;
  const family = CODEC_FAMILY[four];
  if (!family) return true;
  try {
    if (typeof MediaSource === 'undefined' || !MediaSource.isTypeSupported) return true;
    return MediaSource.isTypeSupported('video/mp4; codecs="' + family + '"');
  } catch (err) {
    return true;
  }
}

function stopWatchdog(slide) {
  if (!slide._watchdog) return;
  clearInterval(slide._watchdog);
  clearTimeout(slide._watchdog);
  slide._watchdog = null;
}

// 编码不支持：提示一下再自动跳到下一屏，避免推荐流里一堆黑屏卡片
function skipUnplayable(slide, four, stalled) {
  if (slide._skipping || slide !== activeSlide) return;
  slide._skipping = true;
  stopWatchdog(slide);

  const hint = slide.querySelector('.hint');
  if (hint) {
    let reason;
    if (stalled) reason = '视频加载不出来';
    else if (four) reason = '该视频编码（' + (CODEC_LABEL[four] || four) + '）浏览器不支持';
    else reason = '该视频编码浏览器不支持';
    hint.textContent = reason + '，已跳过';
    hint.classList.add('show');
  }

  const video = slide.querySelector('video');
  if (video) {
    if (video._player) {
      try {
        video._player.destroy();
      } catch (err) {
        // 播放器可能已销毁
      }
      video._player = null;
    }
    destroyHls(video);
    pauseVideo(video);
  }

  setTimeout(() => {
    // 期间用户可能自己滑走了，就别再强行跳
    if (slide !== activeSlide) return;
    if (typeof slide._index === 'number' && slide._index + 1 < dataSource.length) {
      jumpToIndex(slide._index + 1);
    }
  }, 1000);
}

// 卡死看门狗，三种判死方式：
// 1）时间在走却一帧都没有（视频轨解不了，只剩音频在放，进度条却照跑）——2 秒确诊；
// 2）元数据（时长）已解析，再等 5 秒仍无画面——判死；
// 3）从头到尾毫无动静（网络/上游卡住）——12 秒判死。
function armWatchdog(slide, video) {
  stopWatchdog(slide);
  const started = Date.now();
  let metaAt = 0;
  slide._watchdog = setInterval(() => {
    if (slide !== activeSlide) {
      stopWatchdog(slide);
      return;
    }
    // 已经有画面了（loadeddata 或解出尺寸），说明编码没问题
    if (slide.classList.contains('playing') || video.videoWidth > 0) {
      stopWatchdog(slide);
      return;
    }
    // 时间在走却没有画面：视频轨解不了，只剩音频在播
    if (video.currentTime > 1.5) {
      stopWatchdog(slide);
      skipUnplayable(slide, slide._codec, false);
      return;
    }
    // 元数据（时长）都拿到了，再等 5 秒还是没画面，同样判死
    if (video.readyState >= 1 && !metaAt) metaAt = Date.now();
    if (metaAt && Date.now() - metaAt > 5000) {
      stopWatchdog(slide);
      skipUnplayable(slide, slide._codec, false);
      return;
    }
    if (Date.now() - started > 12000) {
      stopWatchdog(slide);
      skipUnplayable(slide, slide._codec, true);
    }
  }, 400);
}

// 装配某一屏：只在第一次真正装载视频，之后复用同一个播放器/数据源。
async function prepareSlide(slide) {
  if (slide.dataset.ready === '1') return;
  slide.dataset.ready = '1';
  slide._skipping = false;
  slide._codecBad = false;

  const video = slide.querySelector('video');
  const hint = slide.querySelector('.hint');
  if (!video) return;

  video._url = '/api/video/stream?path=' + encodeURIComponent(slide.dataset.path);
  video.preload = 'auto';
  hint.textContent = '加载中…';
  hint.classList.add('show');
  // 预读阶段可能已经缓冲到首帧（readyState≥2），此时不会再触发 loadeddata，立即收起提示
  if (video.readyState >= 2) hint.classList.remove('show');

  // 优先用预热阶段已经探好的容器类型（首屏预热会把 Promise 存在 warmContainers 里），
  // 没有再现场探一次
  const container =
    slide._container ||
    (await (warmContainers.get(slide.dataset.path) || sniffContainer(video._url)));
  warmContainers.delete(slide.dataset.path);
  // 探测期间这一屏可能已经被释放（例如快速滑过去了）
  if (slide.dataset.ready !== '1' || slide.dataset.active !== '1') {
    slide.dataset.ready = '0';
    return;
  }
  slide._container = container;

  // m3u8：交给 HLS.js（分片已由后端代理改写成本服务地址）
  if (container === 'hls') {
    loadedSlides.add(slide);
    startHls(video, slide, true);
    armWatchdog(slide, video);
    return;
  }

  if (container === 'ts') {
    loadedSlides.add(slide);
    startMpegts(video, slide, true);
    armWatchdog(slide, video);
    return;
  }
  video.preload = 'auto';
  // 预读阶段可能已经挂了 src，避免重复赋值触发重新加载
  if (!video.getAttribute('src')) video.src = video._url;
  loadedSlides.add(slide);
  playVideo(video);
  armWatchdog(slide, video);

  // 原生 MP4 再验一次编码：mp4v / HEVC 只有时长能解析，画面永远出不来。
  // 探测要下载最多 512KB，如果一开始就跑会和首帧抢带宽、拖慢起播，
  // 所以等「已经出画面」或 1.5 秒后（坏片源早点判死）再跑，谁先到算谁。
  const url = video._url;
  const runProbe = () => {
    if (slide._probeStarted) return;
    slide._probeStarted = true;
    clearTimeout(slide._probeTimer);
    video.removeEventListener('playing', runProbe);
    probeCodec(url).then((four) => {
      slide._codec = four;
      if (slide.dataset.active !== '1' || slide._skipping) return;
      if (codecPlayable(four)) return;
      slide._codecBad = true;
      skipUnplayable(slide, four, false);
    });
  };
  video.addEventListener('playing', runProbe);
  slide._probeTimer = setTimeout(runProbe, 1500);
}

function streamUrl(slide) {
  return '/api/video/stream?path=' + encodeURIComponent(slide.dataset.path);
}

// 1KB Range 探测：提前建好服务端直链缓存/连接，并把容器类型记到这一屏上。
function warmSlide(slide) {
  if (slide.dataset.warm === '1') return Promise.resolve(slide._container || 'unknown');
  slide.dataset.warm = '1';
  return fetch(streamUrl(slide), { headers: { Range: 'bytes=0-2047' } })
    .then((res) => readFirstChunk(res).then((bytes) => {
      slide._container = containerOf(bytes);
      // 编码探测（最多 512KB）不在这里做：预读时当前屏还在起播，
      // 一跑就会抢带宽；等这一屏真正滑到、出画面后再验
      return slide._container;
    }))
    .catch(() => slide._container || 'unknown');
}

// 彻底释放某一屏占用的网络连接和内存。
function releaseSlide(slide) {
  slide.dataset.ready = '0';
  slide.dataset.active = '';
  // 封面恢复显示：这一屏再被滑回来时要先从封面开始，等首帧出来再淡出
  slide.classList.remove('playing');
  slide.classList.remove('painted');
  stopWatchdog(slide);
  slide._skipping = false;

  const video = slide.querySelector('video');
  if (!video) return;
  destroyHls(video);
  if (video._player) {
    try {
      video._player.destroy();
    } catch (err) {
      // 播放器可能已经销毁
    }
    video._player = null;
  }
  if (!video.paused) video.pause();
  video.removeAttribute('src');
  try {
    video.load();
  } catch (err) {
    // 空源 load 在个别浏览器会抛错，忽略
  }
  video._url = '';
  video._fallbackTried = false;
  loadedSlides.delete(slide);
}

// 统一执行白名单：除了「当前屏 + 预读范围内的屏」，其余持有一律释放。
// IntersectionObserver 同一批回调会先 deactivate 旧屏再 activate 新屏，
// 只看 activeSlide 判断会漏掉旧屏，所以这里用已加载集合兜底。
function enforceWhitelist() {
  loadedSlides.forEach((slide) => {
    if (slide !== activeSlide && !preloadSlides.has(slide)) releaseSlide(slide);
  });
}

// 撤销全部预读位：把预读范围内各屏的 src 清空，只剩当前视频持有一条连接。
function releasePreload() {
  preloadSlides.forEach((slide) => {
    if (slide !== activeSlide) releaseSlide(slide);
  });
  preloadSlides.clear();
}

// 预读当前屏后面的 preloadCount 屏（数量在「设置 · 高级设置」里可调，默认 1）。
// MP4 用 preload='metadata' 只下载文件头；TS 必须走 MSE，提前建连会一直拉流，
// 所以只做 1KB 探测，等真正滑到再交给 mpegts.js。
async function preloadNext() {
  if (fastScrolling || !activeSlide) return;

  // 收集后面连续 preloadCount 个 slide（跳过目录边界之外的兄弟节点）
  const targets = [];
  let node = activeSlide.nextElementSibling;
  while (node && targets.length < preloadCount) {
    if (node.classList && node.classList.contains('slide')) targets.push(node);
    node = node.nextElementSibling;
  }

  // 不在新范围里的预读屏立刻释放：改小数量或滑走时都不会留着连接
  [...preloadSlides].forEach((slide) => {
    if (!targets.includes(slide)) {
      releaseSlide(slide);
      preloadSlides.delete(slide);
    }
  });

  for (const next of targets) {
    preloadSlides.add(next);

    const video = next.querySelector('video');
    if (!video || video.getAttribute('src') || video._player) continue;

    const container = await warmSlide(next);
    // 探测期间用户可能已经滑走、换了范围，或开始快速滚动
    if (fastScrolling || !preloadSlides.has(next)) continue;
    if (container !== 'mp4') continue;

    video.preload = 'metadata';
    video.src = streamUrl(next);
    loadedSlides.add(next);
  }
}

// 滑动速度检测：100ms 内累计滚过一屏就判定为快速滑动，立刻掐掉预读。
// 停止滚动 300ms 后才能恢复，按设置的预加载数量重新挂上预读。
let lastScrollTop = 0;
let scrollDistance = 0;
let scrollWindowTimer = null;
let scrollEndTimer = null;

function bindScrollGuard() {
  feedEl.addEventListener(
    'scroll',
    () => {
      const top = feedEl.scrollTop;
      scrollDistance += Math.abs(top - lastScrollTop);
      lastScrollTop = top;

      clearTimeout(scrollWindowTimer);
      scrollWindowTimer = setTimeout(() => {
        scrollDistance = 0;
      }, 100);

      // 100ms 窗口内累计滚过一屏即视为快速滑动（含刚好一屏的快速甩动）
      if (scrollDistance >= (feedEl.clientHeight || 1)) {
        fastScrolling = true;
        releasePreload();
      }

      clearTimeout(scrollEndTimer);
      scrollEndTimer = setTimeout(() => {
        scrollDistance = 0;
        if (fastScrolling) {
          fastScrolling = false;
          // 停止滑动后补回当前屏的播放与下一屏的预读
          if (activeSlide) {
            const video = activeSlide.querySelector('video');
            if (video && !video.paused) playVideo(video);
          }
        }
        preloadNext();
      }, 300);
    },
    { passive: true }
  );
}

function activate(slide) {
  const video = slide.querySelector('video');
  if (!video) return;
  // 预读过的那一屏被滑到了，从预读集合里移除，由当前屏接管
  preloadSlides.delete(slide);
  activeSlide = slide;
  // 先把上一屏及范围外的预读屏释放掉
  enforceWhitelist();
  // 滚动到哪一屏就同步状态机，供「进入合集」时记录 returnIndex
  if (typeof slide._index === 'number') currentPlayIndex = slide._index;
  slide.dataset.active = '1';
  prepareSlide(slide);
  playVideo(video);
  preloadNext();
}

function deactivate(slide) {
  slide.dataset.active = '';
  const video = slide.querySelector('video');
  if (video) pauseVideo(video);
  // 不在「当前屏 + 预读范围」白名单里的，立刻释放
  enforceWhitelist();
}

// ---------- 点赞 ----------
function bump(el) {
  el.classList.remove('bump');
  void el.offsetWidth;
  el.classList.add('bump');
  setTimeout(() => el.classList.remove('bump'), 320);
}

function popHeart(x, y) {
  const heart = document.createElement('div');
  heart.className = 'heart-pop';
  heart.innerHTML = ICONS.heartFill;
  heart.style.left = x + 'px';
  heart.style.top = y + 'px';
  heartLayer.appendChild(heart);
  setTimeout(() => heart.remove(), 950);
}

function likeSlide(slide, x, y) {
  popHeart(x, y);
  if (slide._liked) return;
  slide._liked = true;

  const btn = slide.querySelector('.act-like');
  if (!btn) return;
  btn.classList.add('is-liked');
  const wrap = btn.querySelector('.act-icon');
  if (wrap) wrap.innerHTML = ICONS.heartFill;
  slide._mock.likes += 1;
  const count = btn.querySelector('.act-count');
  if (count) count.textContent = formatCount(slide._mock.likes);
  bump(btn);
}

// 单击暂停/播放，300ms 内同位置再点一次就算双击点赞（桌面鼠标和触屏都适用）。
let tapTimer = 0;
let lastTap = { time: 0, x: 0, y: 0 };

feedEl.addEventListener('click', (event) => {
  const slide = event.target.closest('.slide');
  if (!slide) return;
  // 互动栏、信息区、进度条上的点击交给各自的按钮处理
  if (event.target.closest('.rail, .info, .scrub')) return;

  const now = Date.now();
  const near =
    Math.abs(event.clientX - lastTap.x) < 40 && Math.abs(event.clientY - lastTap.y) < 40;
  if (now - lastTap.time < 300 && near) {
    clearTimeout(tapTimer);
    lastTap.time = 0;
    likeSlide(slide, event.clientX, event.clientY);
    return;
  }

  lastTap = { time: now, x: event.clientX, y: event.clientY };
  const video = slide.querySelector('video');
  clearTimeout(tapTimer);
  tapTimer = setTimeout(() => {
    if (!video) return;
    if (video.paused) {
      playVideo(video);
    } else {
      // 记下「这次暂停是用户点的」，别被取消静音后的自动复查又给播回来
      video._userPaused = true;
      pauseVideo(video);
    }
  }, 260);
});

// ---------- 合集面板 ----------
// 卡片标题：第 X 集，文件名去掉扩展名后如果不是纯序号就作为副标题
function episodeTitle(ep, index) {
  // 分季合集按「本季第几集」编号（各季的第 1 集都是第 1 集），未分季时从头顺数
  const ordinal = ep.season ? ep.seasonIndex + 1 : index + 1;
  const label = '第 ' + ordinal + ' 集';
  const base = (ep.name || '').replace(/\.[^.]+$/, '').trim();
  if (!base) return label;
  if (/^\d+$/.test(base) || base === String(ordinal).padStart(2, '0')) return label;
  return label + ' · ' + base;
}

// 灰色描述：mock 时长 + 清晰度
function episodeDesc(index) {
  const minutes = 3 + ((index * 3) % 8);
  const seconds = (index * 17) % 60;
  return minutes + ':' + String(seconds).padStart(2, '0') + ' · 高清';
}

function renderPlaylist(col, current) {
  sheetTitle.textContent = col.name;
  // 分季合集：副标题带上总季数，单季合集带上季名
  const seasonTotal = col.seasonTotal || 0;
  let sub = '共 ' + col.episodes.length + ' 集';
  if (seasonTotal > 1) {
    sub += ' · ' + seasonTotal + ' 季';
  } else if (seasonTotal === 1 && col.episodes[0] && col.episodes[0].season) {
    sub = col.episodes[0].season + ' · ' + sub;
  }
  sheetSub.textContent = sub;
  sheetList.innerHTML = '';

  let lastSeason = '';
  col.episodes.forEach((ep, index) => {
    // 分季合集的列表按季分块，每季第一集前插一行季标题
    if (seasonTotal > 1 && ep.season && ep.season !== lastSeason) {
      const head = document.createElement('div');
      head.className = 'episode-season';
      head.textContent = ep.season + ' · 共 ' + ep.seasonCount + ' 集';
      sheetList.appendChild(head);
    }
    lastSeason = ep.season || '';

    const item = document.createElement('button');
    item.type = 'button';
    item.className = 'episode-item' + (index === current ? ' current' : '');

    const thumb = document.createElement('img');
    thumb.className = 'episode-thumb';
    thumb.alt = '';
    thumb.loading = 'lazy';
    thumb.src = ep.cover || '';

    const main = document.createElement('div');
    main.className = 'episode-main';
    const title = document.createElement('div');
    title.className = 'episode-title';
    title.textContent = episodeTitle(ep, index);
    const desc = document.createElement('div');
    desc.className = 'episode-desc';
    // 分季时用季内序号编 mock 时长，同一集的时长不会随季变化
    desc.textContent = episodeDesc(ep.season ? ep.seasonIndex : index);
    main.appendChild(title);
    main.appendChild(desc);

    const right = document.createElement('div');
    right.className = 'episode-play';
    if (index === current) {
      // 正在播放：几条跳动的竖线
      const bars = document.createElement('span');
      bars.className = 'playing-bars';
      for (let i = 0; i < 4; i += 1) bars.appendChild(document.createElement('i'));
      right.appendChild(bars);
    } else {
      const play = document.createElement('span');
      play.className = 'pl-play';
      play.innerHTML = ICONS.play;
      right.appendChild(play);
    }

    item.appendChild(thumb);
    item.appendChild(main);
    item.appendChild(right);

    item.addEventListener('click', () => {
      closeSheet();
      // 已经在这个合集里连播：直接滚到目标集，不重建 DOM，切换更顺滑
      if (currentMode === 'collection' && activeCollection && activeCollection.id === col.id) {
        if (index === currentPlayIndex) return;
        jumpToIndex(index);
        return;
      }
      enterCollection(col, index);
    });
    sheetList.appendChild(item);
  });
}

// 把「正在播放」的那一集滚到面板可视区中间。必须在面板显示之后再调用，
// 因为 .sheet[hidden] 时是 display:none，没有布局，scrollIntoView 不会生效。
function scrollToCurrentEpisode() {
  const currentEl = sheetList.querySelector('.episode-item.current');
  if (currentEl) currentEl.scrollIntoView({ block: 'center' });
}

function openSheet(slide) {
  if (!slide || !collections.length) return;
  const col = getCollection(slide._collectionId);
  if (!col || !col.episodes.length) return;
  renderPlaylist(col, slide._episodeIndex || 0);
  sheet.hidden = false;
  requestAnimationFrame(() => {
    sheet.classList.add('open');
    scrollToCurrentEpisode();
  });
}

function closeSheet() {
  sheet.classList.remove('open');
  setTimeout(() => {
    sheet.hidden = true;
  }, 320);
}

sheetClose.addEventListener('click', closeSheet);
sheet.querySelector('.sheet-mask').addEventListener('click', closeSheet);

// ---------- 模式切换：推荐流 <-> 合集连播 ----------

// 进入合集：记住主 Feed 的位置，换成该合集的正序列表，并跳到指定集
function enterCollection(col, episodeIndex) {
  if (currentMode === 'feed') returnIndex = currentPlayIndex;
  activeCollection = col;
  collectionList = col.episodes;

  renderList(collectionList, 'collection');
  jumpToIndex(episodeIndex);
  setBackVisible(true);
}

// 退出合集：换回打乱的推荐流，并回到进入前停留的那一屏
function exitCollection() {
  if (currentMode !== 'collection') return;
  activeCollection = null;
  collectionList = [];

  renderList(feedList, 'feed');
  jumpToIndex(returnIndex);
  setBackVisible(false);
}

function setBackVisible(visible) {
  if (backBtn) backBtn.hidden = !visible;
}

if (backBtn) backBtn.addEventListener('click', exitCollection);

// ---------- 顶部 关注 / 推荐 ----------
function setNavTab(mode) {
  navMode = mode;
  navTabs.forEach((tab) => tab.classList.toggle('active', tab.dataset.nav === mode));
  navEmpty.hidden = mode !== 'follow';

  if (mode === 'follow') {
    feedEl.querySelectorAll('video').forEach(pauseVideo);
    return;
  }
  if (activeSlide) activate(activeSlide);
}

navTabs.forEach((tab) =>
  tab.addEventListener('click', () => {
    const again = tab.classList.contains('active');
    setNavTab(tab.dataset.nav);
    // 已经在「推荐」时再点一次 = 换一批视频（同下拉刷新），不用非得往下滑
    if (again && tab.dataset.nav === 'recommend' && !pageHome.hidden) {
      startRefresh();
    }
  })
);

// ---------- 底部 首页 / 我的 ----------
function switchTab(name) {
  tabButtons.forEach((tab) => tab.classList.toggle('active', tab.dataset.tab === name));
  pageHome.hidden = name !== 'home';
  pageMe.hidden = name !== 'me';
  // 切到「首页/我的」时，设置抽屉、服务器配置、全部合集这些子页都要收起
  closeDrawer();
  pageConfig.hidden = true;
  pageCollections.hidden = true;

  if (name === 'home') {
    if (navMode === 'recommend' && activeSlide) activate(activeSlide);
    return;
  }
  feedEl.querySelectorAll('video').forEach(pauseVideo);
  loadServerSummary();
}

tabButtons.forEach((tab) =>
  tab.addEventListener('click', () => {
    const again = tab.classList.contains('active') && !pageHome.hidden;
    switchTab(tab.dataset.tab);
    // 已经在首页时再点一次「首页」= 刷新推荐流
    if (again && tab.dataset.tab === 'home') startRefresh();
  })
);

// ---------- 我的 · 全部合集 ----------
// 「收录合集」点开后列出所有合集，选一个就从第 1 集开始连播，
// 不用在推荐流里一屏屏刷着找。
function renderCollections() {
  collectionsList.innerHTML = '';
  if (!collections.length) {
    const tip = document.createElement('div');
    tip.className = 'collection-empty';
    tip.textContent = '还没有合集，先下拉刷新一下首页';
    collectionsList.appendChild(tip);
    return;
  }

  collections.forEach((col) => {
    const card = document.createElement('button');
    card.type = 'button';
    card.className = 'collection-card';

    // 没有专属封面，用第一集的缩略图做合集封面
    const cover = document.createElement('img');
    cover.className = 'collection-card-cover';
    cover.alt = '';
    cover.loading = 'lazy';
    const first = col.episodes[0];
    if (first && first.cover) cover.src = first.cover;

    const main = document.createElement('div');
    main.className = 'collection-card-main';
    const title = document.createElement('div');
    title.className = 'collection-card-title';
    title.textContent = col.name;
    const sub = document.createElement('div');
    sub.className = 'collection-card-sub';
    let subText = '共 ' + col.episodes.length + ' 集';
    if (col.seasonTotal > 1) subText += ' · ' + col.seasonTotal + ' 季';
    // 单季合集把季名也写出来，点进去前就知道是哪一季
    if (col.seasonTotal === 1 && first && first.season) subText += ' · ' + first.season;
    sub.textContent = subText;
    main.appendChild(title);
    main.appendChild(sub);

    const arrow = document.createElement('span');
    arrow.className = 'collection-card-arrow';
    arrow.innerHTML = ICONS.play;

    card.appendChild(cover);
    card.appendChild(main);
    card.appendChild(arrow);

    card.addEventListener('click', () => {
      pageCollections.hidden = true;
      switchTab('home');
      enterCollection(col, 0);
    });
    collectionsList.appendChild(card);
  });
}

function openCollectionsPage() {
  renderCollections();
  pageCollections.hidden = false;
  pageMe.hidden = true;
}

function closeCollectionsPage() {
  pageCollections.hidden = true;
  pageMe.hidden = false;
}

meCollectionsCard.addEventListener('click', openCollectionsPage);
collectionsBack.addEventListener('click', closeCollectionsPage);

// ---------- 下拉刷新（重置并打乱推荐流） ----------
const PULL_THRESHOLD = 80; // 松手时超过这个位移才真正刷新
const PULL_MAX = 140; // 最大可视位移
const PULL_RESIST = 0.55; // 阻尼系数：手指走的距离只有一半左右体现在界面上

let pullStartY = -1;
let pullDistance = 0;
let pulling = false;
let refreshing = false;
let wheelAccum = 0;
let wheelTimer = 0;

// 只有「首页 + 推荐流 + 停在第一个视频 + 没在刷新」才允许下拉
function pullAllowed() {
  return !refreshing && !pageHome.hidden && currentMode === 'feed' && feedEl.scrollTop <= 0;
}

// 把当前下拉距离画到提示层上：跟随下移、轻微旋转，到阈值变「松开刷新」
function paintPull(distance) {
  pullDistance = Math.min(distance, PULL_MAX);
  const ready = pullDistance >= PULL_THRESHOLD;
  pullEl.classList.add('show');
  pullEl.classList.toggle('ready', ready);
  pullEl.style.transform = 'translateY(' + (-40 + pullDistance * 0.8).toFixed(1) + 'px)';
  pullEl.style.setProperty('--rot', pullDistance * 3 + 'deg');
  pullText.textContent = ready ? '松开刷新' : '下拉刷新';
}

// 松手但没到阈值：让提示层自己淡出回位
function settlePull() {
  pulling = false;
  pullDistance = 0;
  pullEl.classList.remove('show', 'ready', 'loading');
  pullEl.style.transform = '';
  pullEl.style.removeProperty('--rot');
  pullText.textContent = '下拉刷新';
}

// 真正刷新：停播释放 → 重新打乱 → 重建 DOM → 回到第一个视频
function startRefresh() {
  if (refreshing) return;
  if (!collections.length) {
    settlePull();
    return;
  }

  refreshing = true;
  pullDistance = 0;
  pullEl.classList.add('show', 'loading');
  pullEl.classList.remove('ready');
  pullEl.style.transform = 'translateY(10px)';
  pullEl.style.removeProperty('--rot');
  pullText.textContent = '正在刷新';
  refreshBtn.classList.add('spinning');

  // 合集连播中触发刷新：先退出合集，回到推荐流
  if (currentMode === 'collection') {
    activeCollection = null;
    collectionList = [];
    setBackVisible(false);
  }

  // 1. 停掉当前播放，清空所有 video 的 src 并 load()，释放网络与内存
  teardownSlides();

  // 2. 用现有视频重新打乱：按「根目录下的一级子目录」轮流排布，
  //    刷新后前面的视频来自不同目录，不会只刷到固定的那几部
  feedList = interleaveByDirectory(
    collections.flatMap((col) => col.episodes),
    currentDir
  );

  // 3. 重建整个列表的 DOM，并瞬间回到第一个视频
  //    先预热首屏：把探测和取直链提前发出，与 DOM 构建并行
  warmContainer(feedList[0].path);
  renderList(feedList, 'feed');
  feedEl.scrollTop = 0;
  jumpToIndex(0);

  // 4. 至少展示一小会儿转圈，避免一闪而过
  setTimeout(() => {
    refreshing = false;
    refreshBtn.classList.remove('spinning');
    settlePull();
  }, 800);
}

// 手机：顶部继续往下拖拽时阻止浏览器默认刷新，并给出阻尼位移
feedEl.addEventListener(
  'touchstart',
  (event) => {
    pullStartY = -1;
    if (!pullAllowed() || event.touches.length !== 1) return;
    pullStartY = event.touches[0].clientY;
  },
  { passive: true }
);

feedEl.addEventListener(
  'touchmove',
  (event) => {
    if (pullStartY < 0 || refreshing || event.touches.length !== 1) return;
    if (pageHome.hidden || currentMode !== 'feed') return;

    const dy = event.touches[0].clientY - pullStartY;
    if (dy <= 0) {
      // 手指往上推：正常滚动，收掉已经露出的提示
      if (pulling) settlePull();
      return;
    }
    if (feedEl.scrollTop > 0) {
      // 已经不是第一屏，交给正常滚动
      pullStartY = -1;
      return;
    }

    // 停在顶部还继续下拉：必须阻止默认行为，否则会被浏览器当成刷新网页
    if (event.cancelable) event.preventDefault();
    pulling = true;
    paintPull(dy * PULL_RESIST);
  },
  { passive: false }
);

feedEl.addEventListener('touchend', () => {
  if (!pulling) return;
  const distance = pullDistance;
  pulling = false;
  if (distance >= PULL_THRESHOLD) startRefresh();
  else settlePull();
});

feedEl.addEventListener('touchcancel', () => {
  if (pulling) settlePull();
});

// PC：触控板/鼠标滚轮在顶部继续上滚，同样按阻尼累积位移，停手 300ms 后结算
feedEl.addEventListener(
  'wheel',
  (event) => {
    if (!pullAllowed() || event.deltaY >= 0) {
      wheelAccum = 0;
      return;
    }
    if (event.cancelable) event.preventDefault();
    wheelAccum = Math.min(PULL_MAX / PULL_RESIST, wheelAccum + -event.deltaY);
    paintPull(wheelAccum * PULL_RESIST);

    clearTimeout(wheelTimer);
    wheelTimer = setTimeout(() => {
      const distance = pullDistance;
      wheelAccum = 0;
      if (distance >= PULL_THRESHOLD) startRefresh();
      else settlePull();
    }, 300);
  },
  { passive: false }
);

// PC 右上角小圆按钮：效果与下拉相同
refreshBtn.addEventListener('click', startRefresh);

// ---------- 我的 · 服务器配置 ----------
let configOpen = false;
let serverSummaryLoaded = false;

function showToast(text) {
  toastEl.textContent = text;
  toastEl.hidden = false;
  clearTimeout(showToast.timer);
  showToast.timer = setTimeout(() => {
    toastEl.hidden = true;
  }, 1600);
}

function showConfigError(text) {
  cfgError.textContent = text;
  cfgError.hidden = false;
}

function hideConfigError() {
  cfgError.textContent = '';
  cfgError.hidden = true;
}

// 当前配置回显到表单；密码默认隐藏
function applyConfigForm(data) {
  cfgProtocol.value = String(data.protocol || '').toLowerCase() === 'https' ? 'https' : 'http';
  cfgAddress.value = data.host || '';
  cfgPort.value = data.port || '';
  cfgUsername.value = data.username || '';
  cfgPassword.value = data.password || '';
  cfgVideoPath.value = data.base_path || '';
  cfgPassword.type = 'password';
  cfgEye.innerHTML = ICONS.eyeOff;
  cfgEye.setAttribute('aria-label', '显示密码');
}

// 「我的」右上角齿轮 → 设置抽屉，从屏幕右侧滑入（半屏宽 + 毛玻璃）
let drawerTimer = 0;

function openDrawer() {
  loadServerSummary();
  clearTimeout(drawerTimer);
  settingsDrawer.hidden = false;
  // 先显示（此时面板还在屏幕外），下一帧再加 .show 才会触发滑入动画
  requestAnimationFrame(() => settingsDrawer.classList.add('show'));
}

function closeDrawer() {
  if (settingsDrawer.hidden) return;
  settingsDrawer.classList.remove('show');
  clearTimeout(drawerTimer);
  // 等滑出动画（300ms）走完再真正隐藏，避免动画被打断
  drawerTimer = setTimeout(() => {
    settingsDrawer.hidden = true;
  }, 300);
}

// 「设置」→「服务器配置」，逐级返回
async function openConfigPage() {
  hideConfigError();
  closeDrawer();
  pageConfig.hidden = false;
  configOpen = true;
  try {
    applyConfigForm(await api('/api/config'));
  } catch (err) {
    showConfigError('读取当前配置失败：' + err.message);
  }
}

function closeConfigPage() {
  pageConfig.hidden = true;
  configOpen = false;
  // 从抽屉里进来的，返回时把抽屉恢复，保持上下文
  openDrawer();
}

// 「服务器配置」这一栏右侧的小字，进「我的」时刷一次
async function loadServerSummary() {
  if (serverSummaryLoaded) return;
  serverSummaryLoaded = true;
  try {
    const data = await api('/api/config');
    serverSub.textContent = data.host
      ? data.protocol + '://' + data.host + ':' + data.port
      : '未配置';
  } catch (err) {
    serverSub.textContent = '未配置';
  }
}

cfgEye.addEventListener('click', () => {
  const showing = cfgPassword.type === 'text';
  cfgPassword.type = showing ? 'password' : 'text';
  cfgEye.innerHTML = showing ? ICONS.eyeOff : ICONS.eye;
  cfgEye.setAttribute('aria-label', showing ? '显示密码' : '隐藏密码');
});

cfgForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  hideConfigError();

  const payload = {
    protocol: cfgProtocol.value,
    host: cfgAddress.value.trim(),
    port: cfgPort.value.trim(),
    username: cfgUsername.value.trim(),
    password: cfgPassword.value,
    base_path: cfgVideoPath.value.trim(),
  };
  if (!payload.host) {
    showConfigError('请填写服务器地址');
    return;
  }
  if (!/^\d+$/.test(payload.port)) {
    showConfigError('端口必须是数字');
    return;
  }

  cfgSave.disabled = true;
  cfgSave.textContent = '保存中…';
  try {
    const res = await fetch('/api/config', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    const body = await res.json();
    if (body.code !== 0) {
      throw new Error(body.msg || '保存失败');
    }
    serverSub.textContent =
      payload.protocol + '://' + payload.host + ':' + payload.port;
    showToast('保存成功');
    setTimeout(closeConfigPage, 700);
  } catch (err) {
    showConfigError(err.message || '保存失败');
  } finally {
    cfgSave.disabled = false;
    cfgSave.textContent = '保存';
  }
});

meSettingsBtn.addEventListener('click', openDrawer);
drawerClose.addEventListener('click', closeDrawer);
// 点遮罩空白处也能关（点面板内部不关）
settingsDrawer.addEventListener('click', (event) => {
  if (event.target.closest('[data-drawer-close]')) closeDrawer();
});
serverBtn.addEventListener('click', openConfigPage);
// 清除缓存：只清本地临时数据（最近封面等），服务器配置不受影响
clearCacheBtn.addEventListener('click', () => {
  localStorage.removeItem(LAST_COVER_KEY);
  showToast('缓存已清除');
});
aboutBtn.addEventListener('click', () => showToast('启蒙短视频 v1.0'));
cfgBack.addEventListener('click', closeConfigPage);

// 启动时把保存的偏好同步到静音按钮、设置页开关和之后创建的所有视频
applyDefaultMuted(defaultMuted);

// 整行都可点：点一下切换「是否默认静音」，立刻生效并记住
defaultMuteRow.addEventListener('click', () => applyDefaultMuted(!defaultMuted));
muteBtn.addEventListener('click', () => setMuted(!muted));

// 作品 / 私密 / 喜欢：切换高亮（个人页只是标签样式）
meTabs.forEach((tab) =>
  tab.addEventListener('click', () => {
    meTabs.forEach((t) => t.classList.toggle('active', t === tab));
  })
);

// 高级设置：预加载数量（0~5，默认 1），点一次即保存并立刻按新数量预读
applyPreloadCount(preloadCount);
preloadMinus.addEventListener('click', () => applyPreloadCount(preloadCount - 1));
preloadPlus.addEventListener('click', () => applyPreloadCount(preloadCount + 1));

bindScrollGuard();

// 分季目录名：第 1 季 / 第一季 / 第01-02季 / 第 3 部 / 01 第一季 / 04 第四季 / Season 2 / S02
const SEASON_DIR_RE =
  /^(\d+\s*[-.．、]?\s*)?(第\s*[\d一二三四五六七八九十零〇]+(\s*-\s*[\d一二三四五六七八九十零〇]+)?\s*[季部期]|season\s*\d+|s\d+)$/i;

// 计算视频所属合集：默认用视频所在文件夹；
// 如果该文件夹是分季目录，则合集往上取一层，把同一部的各季合成一个合集。
function collectionDirOf(dirPath) {
  const parts = dirPath.split('/').filter(Boolean);
  const last = parts[parts.length - 1] || '';
  if (parts.length > 1 && SEASON_DIR_RE.test(last)) {
    return { key: '/' + parts.slice(0, -1).join('/'), name: parts[parts.length - 2] };
  }
  return { key: dirPath, name: last || '视频合集' };
}

const CN_DIGITS = { 零: 0, 〇: 0, 一: 1, 二: 2, 三: 3, 四: 4, 五: 5, 六: 6, 七: 7, 八: 8, 九: 9 };

// 中文数字转整数（支持到 99：十一 / 二十 / 二十三）
function cnNumberToInt(text) {
  if (/^\d+$/.test(text)) return Number(text);
  let total = 0;
  let cur = 0;
  for (const ch of text) {
    if (ch === '十') {
      total += (cur || 1) * 10;
      cur = 0;
    } else if (CN_DIGITS[ch] !== undefined) {
      cur = CN_DIGITS[ch];
    }
  }
  return total + cur;
}

// 解析视频所在目录的季标签：返回 { label: '第2季', order: 2 }，
// 目录不是分季目录时返回 null。label 去掉目录名前面的序号（"01 第一季" → "第一季"），
// order 用于把各季按真实顺序排好（避免 第10季 排到 第2季 前面）。
function seasonOfDir(dirPath) {
  const parts = dirPath.split('/').filter(Boolean);
  const last = parts[parts.length - 1] || '';
  const match = SEASON_DIR_RE.exec(last);
  if (!match) return null;

  let label = (match[2] || last).replace(/\s+/g, '');
  const numText = (label.match(/[\d一二三四五六七八九十零〇]+/) || [''])[0];
  const order = numText ? cnNumberToInt(numText) : NaN;
  // season 2 / s02 统一写成「第2季」
  if (/^(season|s)\d+/i.test(label)) {
    label = '第' + (Number.isFinite(order) ? order : label.replace(/^(season|s)/i, '')) + '季';
  }
  return { label, order: Number.isFinite(order) ? order : 999 };
}

// 把一个合集里的原始条目按「季 → 路径」排好，并回填季名、季内序号、本季集数
function buildEpisodes(items, collectionId) {
  const entries = items.map((item) => {
    const dirPath = item.path.slice(0, item.path.lastIndexOf('/')) || '/';
    return { item, season: seasonOfDir(dirPath) };
  });

  entries.sort((a, b) => {
    const ao = a.season ? a.season.order : Number.MAX_SAFE_INTEGER;
    const bo = b.season ? b.season.order : Number.MAX_SAFE_INTEGER;
    if (ao !== bo) return ao - bo;
    const ap = a.item.path.toLowerCase();
    const bp = b.item.path.toLowerCase();
    return ap < bp ? -1 : ap > bp ? 1 : 0;
  });

  const seasonCounts = new Map();
  entries.forEach(({ season }) => {
    if (!season) return;
    seasonCounts.set(season.label, (seasonCounts.get(season.label) || 0) + 1);
  });
  const seasonTotal = seasonCounts.size;
  const seasonSeen = new Map();

  return {
    seasonTotal,
    episodes: entries.map(({ item, season }, index) => {
      let seasonIndex = index;
      if (season) {
        seasonIndex = seasonSeen.get(season.label) || 0;
        seasonSeen.set(season.label, seasonIndex + 1);
      }
      return {
        path: item.path,
        name: item.name,
        cover: item.cover_url,
        collectionId,
        episodeIndex: index,
        season: season ? season.label : '',
        seasonIndex,
        seasonCount: season ? seasonCounts.get(season.label) : 0,
        seasonTotal,
      };
    }),
  };
}

async function boot() {
  // 视频目录直接取服务器配置里的 BasePath（URL 上的 path 仅用于临时调试）
  if (!params.get('path')) {
    try {
      const cfg = await api('/api/config');
      if (cfg.base_path) currentDir = cfg.base_path;
    } catch (err) {
      // 配置接口不可用时继续用内置默认目录
    }
  }

  try {
    const data = await api('/api/feed?path=' + encodeURIComponent(currentDir));
    if (!Array.isArray(data) || data.length === 0) {
      meCount.textContent = '0';
      meWorks.textContent = '0';
      meCollections.textContent = '0';
      showEmpty('该目录下没有视频文件');
      return;
    }

    // 按视频所属合集分组：一个文件夹 = 一个合集（根目录「启蒙」下会有多个合集）。
    // 但文件夹名是「第 1 季」这类分季目录时，合集往上取一层，同一部的各季合并。
    const groups = new Map();
    data.forEach((item) => {
      const dirPath = item.path.slice(0, item.path.lastIndexOf('/')) || '/';
      const { key, name } = collectionDirOf(dirPath);
      if (!groups.has(key)) groups.set(key, { name, items: [] });
      groups.get(key).items.push(item);
    });

    let colIndex = 0;
    groups.forEach((group) => {
      const col = {
        id: 'col-' + colIndex,
        name: group.name,
        episodes: [],
      };
      // 同一部的各季合并成一个合集：按季排好序，并给每集标上所属季与季内序号
      const built = buildEpisodes(group.items, col.id);
      col.episodes = built.episodes;
      col.seasonTotal = built.seasonTotal;
      collections.push(col);
      colIndex += 1;
    });

    // 「我的」页面的真实数据：合集数量 + 顶部模糊背景（取第一支视频的封面）
    meCollections.textContent = String(collections.length);
    if (data[0].cover_url) meCover.src = data[0].cover_url;

    // 推荐流：按根目录下的一级子目录轮流排布，每次进入目录顺序都重新随机
    feedList = interleaveByDirectory(
      collections.flatMap((c) => c.episodes),
      currentDir
    );

    // 首屏预热：在构建几百个 slide 之前，先把第一条视频的容器探测发出去
    // （顺带让服务端把网盘直链取好），起播不用干等这一步
    warmContainer(feedList[0].path);

    renderList(feedList, 'feed');
    jumpToIndex(0);
  } catch (err) {
    showEmpty('加载失败：' + err.message);
  }
}

// 先铺占位封面，再开始拉数据
showBootPreview();
boot();
