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
const meCollections = document.getElementById('me-collections');
const meCollectionsCard = document.getElementById('me-collections-card');
const meTabs = document.querySelectorAll('.me-tab');
const meList = document.getElementById('me-list');
const meEmpty = document.getElementById('me-empty');
const meEmptyText = document.getElementById('me-empty-text');
const meClear = document.getElementById('me-clear');
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
const cfgAlistToggle = document.getElementById('cfg-alist-toggle');
const cfgSourcesEl = document.getElementById('cfg-sources');
const cfgAddSource = document.getElementById('cfg-add-source');
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

// 声音开关：默认静音，点左上角按钮解锁；解锁后切视频保持有声
let soundOn = false;
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
let sheetOrigin = 'feed'; // 选集面板从哪打开：'feed' 首页 | 'collections' 我的·收录合集
let collectionOrigin = 'feed'; // 合集连播从哪进入：决定返回时回到首页 / 收录合集 / 我的

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
    // 在线源按栏目分桶（每个栏目就是一个合集）；网盘源仍按一级目录分桶
    const key = item.category_id || item.category
      ? 'cat:' + (item.category_id || item.category)
      : topDirOf(item.path, root);
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
function bindScrub(slide) {
  const videoOf = () => slide._video;
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
    const video = videoOf();
    if (!video) return 0;
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
    const video = videoOf();
    const total = durationOf();
    if (video && total > 0) video.currentTime = ratio * total;
  };

  function refresh() {
    if (dragging) return;
    const video = videoOf();
    if (!video) {
      scrub.classList.add('hide');
      return;
    }
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

  // 视频元素是池里固定的那几个，事件只在池元素上绑一次（见 bindPoolVideo），
  // 这里把刷新函数挂到屏上，池元素回调里按 video._slide 找到对应进度条
  slide._scrubRefresh = refresh;
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

// 胶囊文案：一眼看出「当前是哪一季的第几集」。
// 分季合集写成「小皮大世界 · 第2季 · 第5集」，未分季的保持原来的写法。
function collectionTagText(col, ep, index) {
  let text = col.name;
  if (ep.season) text += ' · ' + ep.season;
  text += ' · 第' + (ep.season ? ep.seasonIndex + 1 : index + 1) + '集';
  // totalCount 是栏目全部条数（面板拉全后写入）；没有时退回已载入的集数
  if (!ep.season) text += ' / 共 ' + (col.totalCount || col.episodes.length) + ' 集';
  return text;
}

// 把某一集的信息（所属合集、第几集、作者、文案、各项计数）写进这一屏。
// ep 直接传这一屏对应的条目：在线栏目单独插入的「面板里的第 N 条」不在
// col.episodes 里，只能靠传入的条目本身取标题与序号。
function applyInfo(slide, col, ep) {
  const index = ep.episodeIndex || 0;
  const mock = mockFor(index);
  slide._episodeIndex = index;
  slide._collectionId = col.id;
  slide._mock = mock;
  // 这一屏对应的视频信息：双击红心 / 观看历史都要整条存进 localStorage
  slide._meta = buildVideoMeta(col, ep);
  // 回显：之前喜欢过的视频，滑到它时红心一出现就是高亮的红色
  slide._liked = isLikedPath(ep.path);

  const tagText = slide.querySelector('.collection-tag-text');
  if (!tagText) return;

  tagText.textContent = collectionTagText(col, ep, index);

  const avatar = slide.querySelector('.avatar');
  if (avatar) avatar.textContent = mock.author.slice(0, 1);
  // 黑胶唱片中间的「头像」也用同一个首字占位
  const vinylCenter = slide.querySelector('.vinyl-center');
  if (vinylCenter) vinylCenter.textContent = mock.author.slice(0, 1);

  slide.querySelector('.author').textContent = '@' + mock.author;
  // 文案位置显示视频真实标题（源里的 vod_name / 网盘文件名）
  slide.querySelector('.caption').textContent = ep.name || col.name;

  const music = '@' + mock.author + ' 创作的原声';
  slide.querySelectorAll('.marquee-inner span').forEach((span) => {
    span.textContent = music;
  });

  slide.querySelectorAll('.act').forEach((btn) => {
    const count = btn.querySelector('.act-count');
    if (count) count.textContent = formatCount(mock[btn.dataset.count]);
  });

  setLikeUI(slide, slide._liked);
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

  // 视频元素不在这里创建：全站只用固定的 3 个池化 <video>，滑到哪一屏就把
  // 池里的元素搬进哪一屏（换 src），这样用户手势解锁过的元素可以一直复用到新内容
  const hint = document.createElement('div');
  hint.className = 'hint';
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

  bindScrub(slide);
  applyInfo(slide, col, item);
  return slide;
}

// ---------- 视频元素池 ----------
// iOS 的自动播放策略是按元素授权的：只有被用户手势「亲自」play() 过的 <video>
// 才能出声。所以全站只保留固定的 3 个元素，滑到哪一屏就把其中一个搬进哪一屏、
// 只换 src。解锁按钮点一次，3 个元素都拿到发声授权，之后换内容也一直有声。
const POOL_SIZE = 3;
const poolVideos = [];

function initVideoPool() {
  for (let i = 0; i < POOL_SIZE; i++) {
    const video = document.createElement('video');
    video.className = 'player';
    video.muted = true;
    video.loop = true;
    video.playsInline = true;
    video.preload = 'none';
    video.setAttribute('playsinline', '');
    bindPoolVideo(video);
    poolVideos.push(video);
  }
}

// 事件全部挂在池元素上，回调里用 video._slide 取「当前宿主屏」：元素搬家后不会错位
function bindPoolVideo(video) {
  video.addEventListener('loadeddata', () => {
    const slide = video._slide;
    if (!slide) return;
    stopWatchdog(slide);
    slide.classList.add('playing');
  });

  // 封面淡出等「首帧真的画出来」（painted），而不是 loadeddata：
  // MSE(TS) 在 loadeddata 时可能还在解码，提前撤封面会露一两秒黑屏
  video.addEventListener('playing', () => {
    const slide = video._slide;
    if (!slide) return;
    slide.classList.add('painted');
    const hint = slide.querySelector('.hint');
    if (hint) hint.classList.remove('show');
    const coverImg = slide.querySelector('img.cover');
    if (coverImg) saveLastCover(coverImg.getAttribute('src'));
    if (isIOS && !video.muted) ensureAudible(slide);
    // 真正播放起来才开始计时：连续播放满 2 秒才记进观看历史
    scheduleHistory(slide);
  });

  // 进度条跟随「当前宿主屏」：池元素搬去哪一屏，就刷新哪一屏的进度条
  ['timeupdate', 'loadedmetadata', 'durationchange'].forEach((type) => {
    video.addEventListener(type, () => {
      const slide = video._slide;
      if (slide && slide._scrubRefresh) slide._scrubRefresh();
    });
  });

  // 暂停态只由视频自身的 play/pause 事件驱动，保证图标和真实状态一致
  video.addEventListener('pause', () => {
    if (video._slide) {
      video._slide.classList.add('paused');
      // 不到 2 秒就暂停/划走：这次不计入观看历史
      cancelHistory(video._slide);
    }
  });
  video.addEventListener('play', () => {
    if (video._slide) video._slide.classList.remove('paused');
  });

  // 原生解码失败时再兜底试一次 mpegts（只在按容器判断为 mp4/unknown 时才会走到原生）
  video.addEventListener('error', () => {
    const slide = video._slide;
    if (!slide) return;
    if (!video._url) return; // 摘下来时会清空 src，这里跳过空源触发的错误
    if (video._hls) return; // HLS 错误交给 hls.js 自己处理
    const hint = slide.querySelector('.hint');
    if (slide._codecBad) {
      skipUnplayable(slide, slide._codec, false);
      return;
    }
    if (video._fallbackTried) {
      slide.classList.remove('playing');
      if (hint) {
        hint.textContent = '播放失败：该视频编码不支持';
        hint.classList.add('show');
      }
      skipUnplayable(slide, slide._codec, !slide._codec);
      return;
    }
    video._fallbackTried = true;
    startMpegts(video, slide, slide.dataset.active === '1');
  });

  // 循环：原生播放靠 loop 属性；mpegts 的 MediaSource 到片尾已 endOfStream，只能重建播放器。
  // HLS 由 hls.js 自己按 loop 续播，这里不重建，避免把 HLS 实例误建成 mpegts 播放器。
  video.addEventListener('ended', () => {
    const slide = video._slide;
    if (!slide) return;
    if (!video._player || video._hls) return;
    video._player.destroy();
    video._player = null;
    startMpegts(video, slide, slide.dataset.active === '1');
  });
}

// 取这一屏的视频：有就复用，没有就从池里挑一个搬进来
function assignVideo(slide) {
  if (slide._video) return slide._video;
  const video = pickPoolVideo(slide);
  if (!video) return null;
  if (video._slide) detachVideo(video._slide);
  // 起播前用封面当 poster：iOS 的 <video> 没有首帧时会画一块白底，闪一下白框
  const coverImg = slide.querySelector('img.cover');
  video.poster = coverImg ? coverImg.getAttribute('src') || '' : '';
  if (video.parentNode) video.parentNode.removeChild(video);
  slide.appendChild(video);
  slide._video = video;
  video._slide = slide;
  video._url = '';
  video._fallbackTried = false;
  video._audioTries = 0;
  video.muted = !soundOn;
  slide._audioUnlocked = false;
  return video;
}

function pickPoolVideo(slide) {
  const free = poolVideos.find((v) => !v._slide);
  if (free) return free;
  // 都在用：抢距离最远、且不在预读范围里的那一屏
  const index = typeof slide._index === 'number' ? slide._index : 0;
  let victim = null;
  let far = -1;
  poolVideos.forEach((v) => {
    const host = v._slide;
    if (!host || host === slide || host === activeSlide) return;
    if (preloadSlides.has(host)) return;
    const d = Math.abs((host._index || 0) - index);
    if (d > far) {
      far = d;
      victim = v;
    }
  });
  return victim;
}

// 把池元素从某一屏摘下来：销毁播放器、清空 src，元素回池里等下一次装载
function detachVideo(slide) {
  const video = slide._video;
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
  video._slide = null;
  slide._video = null;
  slide.dataset.ready = '0';
  slide.classList.remove('playing');
  slide.classList.remove('paused');
  if (video.parentNode === slide) slide.removeChild(video);
  loadedSlides.delete(slide);
}

initVideoPool();

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
  // 池里的元素要保住（下次渲染还要复用），先把它们从各屏摘下来再清空 DOM
  poolVideos.forEach((video) => {
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
    video._fallbackTried = false;
    video._slide = null;
    video.removeAttribute('src');
    try {
      video.load();
    } catch (err) {
      // 忽略空源导致的异常
    }
    if (video.parentNode) video.parentNode.removeChild(video);
  });
  feedEl.innerHTML = '';
}

function renderList(list, mode) {
  teardownSlides();
  dataSource = list;
  currentMode = mode;
  meCount.textContent = String(list.length);

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
    const prevVideo = prev._video;
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

// 自动播放被浏览器拦下时：先临时静音起播，声音留到用户触摸时再恢复。
// 不能在起播后立刻取消静音：iOS / 部分 WebView 在非用户手势里给视频
// 取消静音会直接把视频暂停，表现就是「每一屏都停在暂停状态」。
let pendingGesturePlay = false; // 连静音自动播放都被拦，等用户触摸再放

// 手机浏览器（尤其 iOS「添加到主屏幕」的独立窗口）在页面还没有任何用户手势时，
// 一律不允许有声自动播放。这类环境直接把「无手势起播」当策略静音处理：先静音起播，
// 等下一次手势再还声音。不能依赖 play() 被拒的异步回调——它常常晚于用户手势到达，
// 只挂一次性监听就会永远等不到第二次手势，表现就是「一直静音」。
let hadGesture = false;
const isIOS =
  /iPad|iPhone|iPod/.test(navigator.userAgent) ||
  (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1);
const needsGestureForAudio =
  isIOS ||
  window.matchMedia('(display-mode: standalone)').matches ||
  window.navigator.standalone === true;

// iOS 的 WebKit 只认「用户手势里发起的播放」有声：自动起播的视频就算 muted=false
// 也是静的（左上角图标显示有声音，实际没声），必须重新触发一次播放才会打开音频通道
// （用户实测「点一下暂停再播放」就有声）。所以分两种时机补声音：
//   1. 真·手势里（touchend / click）：直接解锁这一屏，最可靠；
//   2. 起播之后自动重试最多 2 次：滑动刚结束时通常还在手势有效期内。
// 没解锁成功的屏，下一次手势会再补一次，不会一直静音。
function unlockAudio(video) {
  if (!video || !soundOn || video._userPaused) return;
  if (!video._player && !video.src) return;
  if (video.paused) {
    playVideo(video); // 还没起播：直接在手势里起播，有声
    return;
  }
  restoreAudio(video); // 已在播但没声音：重播一次打开音频通道
}

function unlockSlide(slide) {
  if (!slide || slide._audioUnlocked) return;
  const video = slide._video;
  if (!video || (!video._player && !video.src)) return;
  // 用户自己点停的屏不在这里起播：点击处理里会自己播，这里再播会和「点一下暂停/播放」打架
  if (video.paused) return;
  slide._audioUnlocked = true;
  unlockAudio(video);
}

// 当前滚动位置对应的屏（滑动结束后就是即将成为当前屏的那一屏）
function nearestSlide() {
  const slides = feedEl.querySelectorAll('.slide');
  if (!slides.length) return null;
  const h = feedEl.clientHeight || 1;
  const idx = Math.max(0, Math.min(slides.length - 1, Math.round(feedEl.scrollTop / h)));
  return slides[idx];
}

function ensureAudible(slide) {
  const target = slide || activeSlide;
  if (!target || target.dataset.active !== '1') return;
  if (!soundOn || !isIOS || !hadGesture || target._audioUnlocked) return;
  const video = target._video;
  if (!video || (!video._player && !video.src) || video._userPaused) return;
  video._audioTries = (video._audioTries || 0) + 1;
  if (video._audioTries > 2) return;
  unlockAudio(video);
}

// 借一次用户手势把「策略静音」的视频恢复成有声。
// iOS 独立窗口里光把 muted 改成 false 往往仍然没声音（音频通道没打开），
// 必须重新触发一次播放——效果等同用户手动「点一下暂停再播放」。
function restoreAudio(video) {
  const wasPlaying = !video.paused;
  video._policyMuted = false;
  video.muted = false;
  // 已在播的先暂停：紧接着的 play() 才会重新打开音频通道
  if (wasPlaying) pauseVideo(video);
  tryStartVideo(video).catch(() => revertToMuted(video));
  // 有的 WebView 不报错，而是直接把视频暂停：只要不是用户自己点的，退回静音继续播
  setTimeout(() => {
    const slide = video.parentNode;
    if (slide && slide.dataset.active !== '1') return;
    if (!video._policyMuted && video.paused && !video._userPaused) revertToMuted(video);
  }, 400);
}

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

  if (!hadGesture && needsGestureForAudio) {
    // 手势之前的有声自动播放一定会被拦，直接静音起播并标记，等手势再还声音
    video.muted = true;
    video._policyMuted = true;
  } else {
    // 每次起播都按声音开关同步一次：预读过的屏可能还留着上一次的静音状态，
    // 不同步就会出现「切到下一屏就没声音」
    video._policyMuted = false;
    video.muted = !soundOn;
  }

  tryStartVideo(video).catch(() => {
    // 有声自动播放被拦：先静音让它动起来，标记一下，等手势再还声音
    video.muted = true;
    video._policyMuted = true;
    tryStartVideo(video).catch(() => {
      // 极少数 WebView 连静音自动播放都拦：保持暂停，等用户点一下再播
      pendingGesturePlay = true;
    });
  });
}

// 退回静音继续播：声音策略失败后的兜底，保证画面不会停在暂停态
function revertToMuted(video) {
  if (video._policyMuted) return;
  video._policyMuted = true;
  video.muted = true;
  // 解锁没成功：清掉这屏的标记，下一次手势再补一次
  if (video.parentNode) video.parentNode._audioUnlocked = false;
  tryStartVideo(video).catch(() => {});
}

// 每次用户触摸/按键都检查一遍当前屏，把「策略静音」的视频恢复成有声。
// 不能只做一次——自动播放被拦的回调是异步的，常常在用户第一次触摸之后才到达。
// 用 touchend / click：iOS 只认这两级用户手势，touchstart 不算。
// 手势必须落在首页的视频区域里才操作播放：在「我的」、抽屉、合集面板上点按，
// 不能顺手把后台的视频播起来。
function gestureInFeed(event) {
  if (pageHome.hidden || sheet.classList.contains('open')) return false;
  if (!event || event.type === 'keydown') return true;
  const target = event.target;
  if (!target || target === document || target === document.body) return true;
  // 拖进度条、点右侧互动栏时不碰播放，免得重播一下
  if (target.closest && target.closest('.scrub, .rail')) return false;
  return feedEl.contains(target);
}

function onUserGesture(event) {
  hadGesture = true;
  if (!gestureInFeed(event)) return;
  if (pendingGesturePlay && activeSlide) {
    pendingGesturePlay = false;
    const video = activeSlide._video;
    if (video && video.paused) playVideo(video);
  }
  if (!activeSlide || !soundOn) return;
  if (isIOS) {
    // 手势这一拍里解锁当前屏；滑动时顺手解锁即将到位的那一屏
    unlockSlide(activeSlide);
    const near = nearestSlide();
    if (near && near !== activeSlide) unlockSlide(near);
    return;
  }
  ensureAudible();
}

['touchend', 'click', 'keydown'].forEach((type) =>
  document.addEventListener(type, onUserGesture, { capture: true })
);

function pauseVideo(video) {
  if (video._player) {
    video._player.pause();
  } else if (!video.paused) {
    video.pause();
  }
}

// 页面级音频会话解锁：和 play() 放在同一个点击事件里，iOS 才认。
let audioCtx = null;
function unlockAudioContext() {
  try {
    const Ctx = window.AudioContext || window.webkitAudioContext;
    if (!Ctx) return;
    if (!audioCtx) audioCtx = new Ctx();
    if (audioCtx.state === 'suspended') audioCtx.resume();
  } catch (err) {
    // 不支持 AudioContext 的浏览器直接忽略
  }
}

// 声音开关：默认静音，点按钮解锁。
// 解锁必须在手势里「亲自」对每个池元素调用 play()：iOS 的发声授权是按元素给的，
// 这里遍历全部池元素，有内容的一并 play()，之后换 src 也保持有声。
function setSound(next) {
  soundOn = next;
  muteBtn.classList.toggle('is-muted', !soundOn);
  muteBtn.innerHTML = soundOn ? ICONS.volumeOn : ICONS.volumeMute;
  muteBtn.setAttribute('aria-label', soundOn ? '关闭声音' : '开启声音');

  if (!soundOn) {
    poolVideos.forEach((video) => {
      video.muted = true;
      video._policyMuted = false;
    });
    return;
  }

  unlockAudioContext();

  const active = activeSlide ? activeSlide._video : null;
  poolVideos.forEach((video) => {
    // 严禁用 volume 淡入：授权只靠 muted=false + play()，由池元素自己拿到
    video._policyMuted = false;
    if (!video._slide || (!video.src && !video._player)) {
      // 还没挂源的元素：只同步状态，等真正起播时 playVideo 会再设一次 muted
      video.muted = false;
      return;
    }
    if (video === active) {
      video.muted = false;
      const started = video.play();
      if (started && started.catch) started.catch(() => {});
      return;
    }
    // 邻屏 / 预读屏：静音 play() 一下只为拿发声授权，随即暂停，避免后台外放
    video.muted = true;
    const started = video.play();
    if (started && started.catch) started.catch(() => {});
    setTimeout(() => {
      if (video._slide && video._slide !== activeSlide && !video.paused) video.pause();
    }, 0);
  });

  if (!active) return;
  if (!active.src && !active._player) return;
  if (active.paused) {
    playVideo(active); // 在手势里起播，直接有声
    return;
  }
  restoreAudio(active); // 已在播：重播一次打开音频通道
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

  const video = slide._video;
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

  const video = assignVideo(slide);
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
  if (!video.getAttribute('src')) {
    video.muted = !soundOn; // 换源后立刻按全局声音状态同步 muted
    video.src = video._url;
  }
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
  // 池元素摘回池里，这一屏不再持有任何连接
  detachVideo(slide);
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

    const video = next._video || assignVideo(next);
    if (!video) continue;
    if (video.getAttribute('src') || video._player) continue;

    const container = await warmSlide(next);
    // 探测期间用户可能已经滑走、换了范围，或开始快速滚动
    if (fastScrolling || !preloadSlides.has(next)) continue;
    if (container !== 'mp4') continue;

    video.preload = 'metadata';
    video.muted = !soundOn; // 换源后立刻按全局声音状态同步 muted
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
            const video = activeSlide._video;
            if (video && !video.paused) playVideo(video);
          }
        }
        preloadNext();
        // iOS：滑动停稳后再补一次有声检查（这时通常还在手势有效期内）
        ensureAudible();
      }, 300);
    },
    { passive: true }
  );
}

function activate(slide) {
  const video = assignVideo(slide);
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
  // 新屏起播后再补一次有声检查：滑动刚结束，这时通常还在手势有效期内
  if (isIOS) {
    slide._audioUnlocked = false;
    video._audioTries = 0;
    if (soundOn && hadGesture) {
      clearTimeout(slide._audibleTimer);
      slide._audibleTimer = setTimeout(() => {
        if (slide.dataset.active === '1') ensureAudible(slide);
      }, 400);
    }
  }
  preloadNext();
}

function deactivate(slide) {
  slide.dataset.active = '';
  cancelHistory(slide);
  const video = slide._video;
  if (video) pauseVideo(video);
  // 不在「当前屏 + 预读范围」白名单里的，立刻释放
  enforceWhitelist();
}

// ---------- 喜欢 / 观看历史（localStorage 持久化） ----------
// 只存前端本地：喜欢列表与观看历史互不影响，清空某个 Tab 只删对应的 key。
const LIKED_KEY = 'liked_videos';
const HISTORY_KEY = 'history_videos';
const HISTORY_MAX = 50;

function readLocalList(key) {
  try {
    const raw = localStorage.getItem(key);
    const list = raw ? JSON.parse(raw) : [];
    return Array.isArray(list) ? list.filter((v) => v && v.path) : [];
  } catch (err) {
    return [];
  }
}

function writeLocalList(key, list) {
  try {
    localStorage.setItem(key, JSON.stringify(list));
  } catch (err) {
    // 隐私模式 / 配额满：静默失败，不影响播放
  }
}

let likedVideos = readLocalList(LIKED_KEY);
const likedPaths = new Set(likedVideos.map((v) => v.path));

function isLikedPath(path) {
  return !!path && likedPaths.has(path);
}

// 一屏对应的视频信息：vod_id / vod_name / vod_pic / category_name 都写全，
// 后面的喜欢列表、观看历史直接拿这条记录渲染网格。
function buildVideoMeta(col, ep) {
  const path = ep.path || '';
  // 在线源的 path 是「src:源ID:视频ID」，取最后一段当 vod_id；网盘条目用路径
  const online = /^src:[^:]+:(.+)$/.exec(path);
  return {
    vod_id: online ? online[1] : path,
    vod_name: ep.name || col.name,
    vod_pic: ep.cover || '',
    category_name: col.name,
    path,
  };
}

// 红心状态只走这一个入口，避免图标 / 计数 / 本地数据三处不同步
function setLikeUI(slide, liked) {
  slide._liked = liked;
  const btn = slide.querySelector('.act-like');
  if (!btn) return;
  btn.classList.toggle('is-liked', liked);
  const wrap = btn.querySelector('.act-icon');
  if (wrap) wrap.innerHTML = liked ? ICONS.heartFill : ICONS.heartOutline;
}

// 喜欢 / 取消喜欢：同步 localStorage，并把当前所有屏的红心一起刷一遍
function setVideoLiked(meta, liked) {
  if (!meta || !meta.path) return;
  if (liked) {
    if (likedPaths.has(meta.path)) return;
    likedPaths.add(meta.path);
    likedVideos.unshift(meta);
  } else {
    if (!likedPaths.has(meta.path)) return;
    likedPaths.delete(meta.path);
    likedVideos = likedVideos.filter((v) => v.path !== meta.path);
  }
  writeLocalList(LIKED_KEY, likedVideos);
}

// 观看历史：停留播放满 2 秒才记一条；去重、最新的排第一、最多 50 条
function recordHistory(meta) {
  if (!meta || !meta.path) return;
  const entry = Object.assign({}, meta, { watched_at: Date.now() });
  const list = readLocalList(HISTORY_KEY).filter((v) => v.path !== meta.path);
  list.unshift(entry);
  if (list.length > HISTORY_MAX) list.length = HISTORY_MAX;
  writeLocalList(HISTORY_KEY, list);
}

function scheduleHistory(slide) {
  if (slide._historyTimer || slide._historyLogged) return;
  slide._historyTimer = setTimeout(() => {
    slide._historyTimer = 0;
    // 只记录当前屏：预读屏若也起播了，计时到点时不算数
    if (!slide.dataset.active || !slide._meta) return;
    slide._historyLogged = true;
    recordHistory(slide._meta);
  }, 2000);
}

function cancelHistory(slide) {
  if (!slide._historyTimer) return;
  clearTimeout(slide._historyTimer);
  slide._historyTimer = 0;
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

// 双击红心是「开关」：没喜欢就点赞并写入 liked_videos，已喜欢就取消并从数组删除
function likeSlide(slide, x, y) {
  const liked = !slide._liked;
  // 只有点赞冒红心；取消点赞只把图标换回空心
  if (liked) popHeart(x, y);
  setLikeUI(slide, liked);
  setVideoLiked(slide._meta, liked);

  const btn = slide.querySelector('.act-like');
  if (!btn || !slide._mock) return;
  slide._mock.likes = Math.max(0, slide._mock.likes + (liked ? 1 : -1));
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
  const video = slide._video;
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
  // 面板还没拉全时也要显示后端给的真实总数，不能按已加载条数算
  let sub = '共 ' + (col.totalCount || col.episodes.length) + ' 集';
  if (seasonTotal > 1) {
    sub += ' · ' + seasonTotal + ' 季';
  } else if (seasonTotal === 1 && col.episodes[0] && col.episodes[0].season) {
    sub = col.episodes[0].season + ' · ' + sub;
  }
  sheetSub.textContent = sub;
  sheetList.innerHTML = '';
  sheetList.classList.remove('episode-grid');
  sheetList.classList.remove('skeleton-grid');

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
      enterCollection(col, index, sheetOrigin);
    });
    sheetList.appendChild(item);
  });
}

// 把「正在播放」的那一集滚到面板可视区中间。必须在面板显示之后再调用，
// 因为 .sheet[hidden] 时是 display:none，没有布局，scrollIntoView 不会生效。
function scrollToCurrentEpisode() {
  const currentEl = sheetList.querySelector('.episode-item.current, .episode-card.current');
  if (currentEl) currentEl.scrollIntoView({ block: 'center' });
}

function showSheet() {
  sheet.hidden = false;
  requestAnimationFrame(() => {
    sheet.classList.add('open');
    scrollToCurrentEpisode();
  });
}

// 在线栏目的全部内容：按栏目 ID 缓存一份（一次会话内不重复翻页几十次）。
// 失败时不缓存，下次打开面板可以重试。
const categoryAllCache = new Map();

function loadCategoryAll(categoryId, sourceId) {
  const key = (sourceId || '') + ':' + categoryId;
  if (!categoryAllCache.has(key)) {
    let url = '/api/feed/all?category_id=' + encodeURIComponent(categoryId);
    if (sourceId) url += '&source_id=' + encodeURIComponent(sourceId);
    categoryAllCache.set(key, api(url));
  }
  return categoryAllCache.get(key);
}

// 打开某个合集的选集面板。currentPath 用来高亮「正在播放」的那一集，
// 不能用序号：在线栏目首页那 30 条是打乱顺序的，和面板拉全后的顺序不一致。
// slide 只在从首页打开时传入，用于把胶囊文案同步成面板里的真实集号。
function openCollectionPanel(col, currentPath, origin, slide) {
  if (!col) return;
  if (!col.categoryId && !col.episodes.length) return;
  sheetOrigin = origin || 'feed';
  if (col.categoryId) {
    // 在线栏目：先是首页那 30 条，面板里再按需拉全栏目几百条
    openCategorySheet(col, currentPath, slide);
    return;
  }
  const current = currentPath
    ? col.episodes.findIndex((ep) => ep.path === currentPath)
    : -1;
  renderPlaylist(col, current);
  showSheet();
}

function openSheet(slide) {
  if (!slide || !collections.length) return;
  const col = getCollection(slide._collectionId);
  openCollectionPanel(col, slide.dataset.path || '', 'feed', slide);
}

async function openCategorySheet(col, currentPath, slide) {
  sheetTitle.textContent = col.name;
  sheetSub.textContent = '正在加载全部内容…';
  // 数据没回来前先铺骨架屏：15 个灰色闪烁卡片，比「加载中…」更不跳
  sheetList.classList.remove('episode-grid');
  sheetList.classList.add('skeleton-grid');
  sheetList.innerHTML = '';
  sheetList.appendChild(buildSkeletonGrid());
  showSheet();

  try {
    const items = await loadCategoryAll(col.categoryId, col.sourceId);
    // 后端 /api/feed 已给真实总数时保持不变，只有没拿到时才按拉全的条数兜底
    if (!col.totalCount) col.totalCount = items.length;
    let current = -1;
    if (currentPath) {
      current = items.findIndex((item) => item.path === currentPath);
      // 首页上的集号是按打乱顺序算的，这里拿到了栏目的真实顺序，
      // 顺手把正在播放那一屏的胶囊文案改成真实集号。
      if (slide && current >= 0) syncSlideEpisode(slide, col, items[current], current);
    }
    renderCategoryGrid(col, items, current);
    requestAnimationFrame(scrollToCurrentEpisode);
  } catch (err) {
    categoryAllCache.delete((col.sourceId || '') + ':' + col.categoryId);
    // 拉全失败时退回首页已载入的那一页，至少还能选集
    const current = currentPath
      ? col.episodes.findIndex((ep) => ep.path === currentPath)
      : -1;
    renderPlaylist(col, current);
    const note = document.createElement('div');
    note.className = 'sheet-loading';
    note.textContent = '全部内容加载失败：' + err.message;
    sheetList.prepend(note);
  }
}

// 把一屏的集号与胶囊文案改成面板里的真实序号（不重建这一屏）。
function syncSlideEpisode(slide, col, ep, index) {
  slide._episodeIndex = index;
  const tagText = slide.querySelector('.collection-tag-text');
  if (tagText) tagText.textContent = collectionTagText(col, ep, index);
}

// 极简骨架屏：数据没回来前先铺一排灰色闪烁卡片，宽高与真实封面一致，
// 数据一到直接整体替换，不会出现布局跳动。
function buildSkeletonGrid(count = 15) {
  const fragment = document.createDocumentFragment();
  for (let i = 0; i < count; i++) {
    const card = document.createElement('div');
    card.className = 'skeleton-card';
    const cover = document.createElement('div');
    cover.className = 'skeleton-cover';
    const line = document.createElement('div');
    line.className = 'skeleton-line';
    card.append(cover, line);
    fragment.appendChild(card);
  }
  return fragment;
}

// 面板网格：几百条只渲染封面 + 标题，图片用浏览器原生懒加载。
function renderCategoryGrid(col, items, current) {
  sheetTitle.textContent = col.name;
  sheetSub.textContent = '共 ' + items.length + ' 集';
  sheetList.innerHTML = '';
  sheetList.classList.remove('skeleton-grid');
  sheetList.classList.add('episode-grid');

  const fragment = document.createDocumentFragment();
  items.forEach((item, index) => {
    const card = document.createElement('button');
    card.type = 'button';
    card.className = 'episode-card' + (index === current ? ' current' : '');

    const thumb = document.createElement('img');
    thumb.className = 'episode-card-cover';
    thumb.loading = 'lazy';
    thumb.decoding = 'async';
    thumb.alt = '';
    // 没封面时不设 src：空 src 会让浏览器额外请求一次本页面
    if (item.cover_url) thumb.src = item.cover_url;

    const title = document.createElement('div');
    title.className = 'episode-card-title';
    title.textContent = item.name;

    card.appendChild(thumb);
    card.appendChild(title);
    card.addEventListener('click', () => playCategoryItem(col, item, items, index));
    fragment.appendChild(card);
  });
  sheetList.appendChild(fragment);
}

// 面板里点某一条：把它的播放数据插到当前屏后面并跳过去，不重建整个栏目。
function playCategoryItem(col, item, items, index) {
  closeSheet();

  // 从「我的 · 收录合集」进来的：把整个栏目当作连播列表，
  // 返回时才能先回到这个合集，再回到我的页面。
  if (sheetOrigin === 'collections') {
    if (!col.totalCount) col.totalCount = items.length;
    enterCollection(
      col,
      index,
      'collections',
      items.map((it, i) => ({
        path: it.path,
        name: it.name,
        cover: it.cover_url,
        collectionId: col.id,
        episodeIndex: i,
        season: '',
        seasonIndex: i,
        seasonCount: 0,
        seasonTotal: col.seasonTotal || 0,
      }))
    );
    return;
  }

  // 首页已载入的那 30 条直接跳过去，不重复插入
  const existed = dataSource.findIndex((ep) => ep.path === item.path);
  if (existed >= 0) {
    // 首页的集号来自打乱后的顺序，用面板里的真实序号覆盖，
    // 否则胶囊里的「第几集」和面板中高亮的那一集仍然对不上。
    dataSource[existed].episodeIndex = index;
    jumpToIndex(existed);
    if (activeSlide) syncSlideEpisode(activeSlide, col, item, index);
    return;
  }

  const ep = {
    path: item.path,
    name: item.name,
    cover: item.cover_url,
    collectionId: col.id,
    episodeIndex: index,
    season: '',
    seasonIndex: index,
    seasonCount: 0,
    seasonTotal: col.seasonTotal || 0,
  };
  if (!col.totalCount) col.totalCount = items.length;
  // 插到当前播放的下一屏：看完这条继续往下滑，仍然是原来的信息流
  const pos = Math.min(currentPlayIndex + 1, dataSource.length);
  dataSource.splice(pos, 0, ep);
  renderList(dataSource, currentMode);
  jumpToIndex(pos);
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

// 进入合集：记住主 Feed 的位置，换成该合集的正序列表，并跳到指定集。
// origin：'collections' 从「我的 · 收录合集」进入，'mine' 从「我的 · 观看历史 / 喜欢」进入，
// 其余按首页推荐流处理；返回时分别回到对应页面。
function enterCollection(col, episodeIndex, origin, list) {
  const fromCollections = origin === 'collections';
  const fromMine = origin === 'mine';
  if (currentMode === 'feed') returnIndex = currentPlayIndex;
  collectionOrigin = fromCollections ? 'collections' : fromMine ? 'mine' : 'feed';

  if (fromCollections) {
    // 从收录合集页进入播放：收起合集页，切回首页承载全屏播放
    pageCollections.hidden = true;
    pageMe.hidden = true;
    pageHome.hidden = false;
    tabButtons.forEach((tab) =>
      tab.classList.toggle('active', tab.dataset.tab === 'home')
    );
  }

  activeCollection = col;
  collectionList = list || col.episodes;

  renderList(collectionList, 'collection');
  jumpToIndex(episodeIndex);
  setBackVisible(true);
}

// 退出合集：换回打乱的推荐流，并回到进入前停留的那一屏。
// 从「我的 · 收录合集」进来的先回到收录合集页并重开选集面板；
// 从「我的 · 观看历史 / 喜欢」进来的回到「我的」页（列表重新渲染，把刚看的这条补进历史）。
function exitCollection() {
  if (currentMode !== 'collection') return;
  const col = activeCollection;
  const fromCollections = collectionOrigin === 'collections';
  const fromMine = collectionOrigin === 'mine';
  collectionOrigin = 'feed';
  activeCollection = null;
  collectionList = [];
  setBackVisible(false);

  // 先切页面再重建：从「我的」退出时首页保持隐藏，
  // 避免 IntersectionObserver 在重建后立刻起播、在后台出声。
  if (fromCollections) {
    pageHome.hidden = true;
    pageMe.hidden = true;
    pageCollections.hidden = false;
    tabButtons.forEach((tab) =>
      tab.classList.toggle('active', tab.dataset.tab === 'me')
    );
  } else if (fromMine) {
    pageHome.hidden = true;
    pageCollections.hidden = true;
    pageMe.hidden = false;
    tabButtons.forEach((tab) =>
      tab.classList.toggle('active', tab.dataset.tab === 'me')
    );
  }

  renderList(feedList, 'feed');
  if (returnIndex >= 0 && returnIndex < dataSource.length) {
    const target = feedEl.querySelectorAll('.slide')[returnIndex];
    if (target) {
      feedEl.scrollTop = target.offsetTop;
      currentPlayIndex = returnIndex;
    }
  }
  feedEl.querySelectorAll('video').forEach(pauseVideo);

  if (fromCollections && col) openCollectionPanel(col, '', 'collections');
  else if (fromMine) renderMineTab();
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

// ---------- 我的 · 观看历史 / 喜欢 ----------
let meTab = 'history';

const ME_EMPTY_TEXT = {
  history: '还没有观看记录',
  likes: '还没有喜欢的视频',
};

// 读取当前 Tab 的本地数据并重绘：B 站风格单列（左图右文）；空数组时显示空状态。
// 只读取不写入：localStorage 里原有的时间戳等字段全部保留，渲染时不画时间。
function renderMineTab() {
  const items = meTab === 'likes' ? likedVideos : readLocalList(HISTORY_KEY);
  meTabs.forEach((tab) => tab.classList.toggle('active', tab.dataset.meTab === meTab));

  const empty = items.length === 0;
  meList.replaceChildren();
  meList.hidden = empty;
  meEmpty.hidden = !empty;
  meClear.hidden = empty;
  meEmptyText.textContent = ME_EMPTY_TEXT[meTab] || ME_EMPTY_TEXT.history;
  if (empty) return;

  const fragment = document.createDocumentFragment();
  items.forEach((item) => {
    const row = document.createElement('button');
    row.type = 'button';
    row.className = 'me-row';
    row.setAttribute('aria-label', item.vod_name || '播放');

    // 左：140x80 横图缩略图；没有封面或加载失败时留深色块
    if (item.vod_pic) {
      const img = document.createElement('img');
      img.className = 'me-row-thumb';
      img.alt = '';
      img.loading = 'lazy';
      img.src = item.vod_pic;
      img.addEventListener('error', () => {
        const ph = document.createElement('div');
        ph.className = 'me-row-thumb';
        img.replaceWith(ph);
      });
      row.appendChild(img);
    } else {
      const ph = document.createElement('div');
      ph.className = 'me-row-thumb';
      row.appendChild(ph);
    }

    // 右：分类标签 + 标题（最多两行），不生成时间节点
    const body = document.createElement('span');
    body.className = 'me-row-body';
    const cat = document.createElement('span');
    cat.className = 'me-row-cat';
    cat.textContent = item.category_name || '';
    const name = document.createElement('span');
    name.className = 'me-row-name';
    name.textContent = item.vod_name || '';
    body.append(cat, name);
    row.appendChild(body);

    row.addEventListener('click', () => playStoredVideo(item));
    fragment.appendChild(row);
  });
  meList.appendChild(fragment);
}

// 点列表整行：切回首页，把这条视频当成只有一集的合集直接开播；
// 返回键回到「我的」页原来的列表，而不是首页推荐流。
function playStoredVideo(item) {
  if (!item || !item.path) return;
  const ep = {
    path: item.path,
    name: item.vod_name || '',
    cover: item.vod_pic || '',
    collectionId: 'mine:' + item.path,
    episodeIndex: 0,
    season: '',
    seasonIndex: 0,
    seasonCount: 0,
    seasonTotal: 1,
  };
  const col = {
    id: ep.collectionId,
    name: item.category_name || '',
    episodes: [ep],
    totalCount: 1,
    seasonTotal: 1,
  };
  switchTab('home');
  enterCollection(col, 0, 'mine', [ep]);
}

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
  renderMineTab();
}

tabButtons.forEach((tab) =>
  tab.addEventListener('click', () => switchTab(tab.dataset.tab))
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
    // 面板拉全过就显示栏目真实总集数，否则先显示首页已载入的条数
    let subText = '共 ' + (col.totalCount || col.episodes.length) + ' 集';
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

    // 点开合集只显示该合集的选集列表，不直接跳进播放；
    // 在列表里选一集后，返回路径才是「播放 → 合集列表 → 我的」。
    card.addEventListener('click', () => {
      openCollectionPanel(col, '', 'collections');
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

// ---------- 数据源配置 ----------
// 在线源草稿：已保存的源 + 刚点「添加数据源」还没保存的源，都放在同一个数组里；
// 输入时就同步回对象，保存时不需要再逐个读 DOM。
let sourceSeq = 0;
const sourceDrafts = [];

function nextSourceUID() {
  sourceSeq += 1;
  return 'src-' + sourceSeq;
}

function newSourceDraft(data) {
  const d = data || {};
  return {
    uid: nextSourceUID(),
    id: String(d.id || ''),
    kind: d.kind === 'js' ? 'js' : 'drpy',
    name: String(d.name || d.source_name || ''),
    enabled: d.enabled !== false,
    categories: Array.isArray(d.categories) ? d.categories.map(String) : [],
    base_url: String(d.base_url || ''),
    source_name: String(d.source_name || ''),
    pwd: String(d.pwd || ''),
    script: String(d.script || ''),
    script_name: String(d.script_name || ''),
    catOptions: [],
    status: '',
    statusKind: '',
    testing: false,
    autoTimer: 0,
    catsLoaded: false,
  };
}

function normalizeCategories(list) {
  if (!Array.isArray(list)) return [];
  return list
    .map((cat) => ({
      id: String((cat && cat.id) !== undefined ? cat.id : ''),
      name: String((cat && cat.name) || (cat && cat.id) || ''),
    }))
    .filter((cat) => cat.id);
}

function isAlistEnabled() {
  return cfgAlistToggle.getAttribute('aria-pressed') !== 'false';
}

function setAlistEnabled(value) {
  cfgAlistToggle.classList.toggle('on', value);
  cfgAlistToggle.setAttribute('aria-pressed', value ? 'true' : 'false');
}

// 只更新这一张卡片的状态文字，不整卡重绘，避免打断用户正在输入的内容
function sourceStatus(src, text, kind) {
  src.status = text;
  src.statusKind = kind || '';
  const card = cfgSourcesEl.querySelector('[data-uid="' + src.uid + '"]');
  if (!card) return;
  const el = card.querySelector('[data-role="status"]');
  if (!el) return;
  el.textContent = text;
  el.className = 'cfg-source-status' + (kind ? ' ' + kind : '');
}

function collectSourceInputs(src, card) {
  const q = (role) => card.querySelector('[data-role="' + role + '"]');
  const name = q('name');
  if (name) src.name = name.value;
  if (src.kind === 'drpy') {
    const base = q('base_url');
    if (base) src.base_url = base.value;
    const sourceName = q('source_name');
    if (sourceName) src.source_name = sourceName.value;
    const pwd = q('pwd');
    if (pwd) src.pwd = pwd.value;
  } else {
    const script = q('script');
    if (script) src.script = script.value;
  }
}

function sourcePayload(src) {
  return {
    id: src.id,
    kind: src.kind,
    name: src.name.trim(),
    enabled: src.enabled,
    categories: src.categories,
    base_url: src.base_url.trim(),
    source_name: src.source_name.trim(),
    pwd: src.pwd.trim(),
    script: src.script,
    script_name: src.script_name,
  };
}

function renderSources() {
  cfgSourcesEl.innerHTML = '';
  if (!sourceDrafts.length) {
    const empty = document.createElement('div');
    empty.className = 'cfg-sources-empty';
    empty.textContent = '还没有在线数据源，点下面的「添加数据源」开始。';
    cfgSourcesEl.appendChild(empty);
    return;
  }
  sourceDrafts.forEach(renderSourceCard);
}

function renderSourceCard(src) {
  const card = document.createElement('div');
  card.className = 'cfg-source-card';
  card.dataset.uid = src.uid;
  cfgSourcesEl.appendChild(card);

  const head = document.createElement('div');
  head.className = 'cfg-source-head';
  head.innerHTML = [
    '<span class="cfg-source-kind">' + (src.kind === 'js' ? 'JS 规则' : 'drpy-node') + '</span>',
    '<button class="switch" type="button" data-role="enabled" aria-label="启用这个数据源"><span class="switch-knob"></span></button>',
    '<button class="cfg-source-del" type="button" data-role="del">删除</button>',
  ].join('');

  const nameField = document.createElement('label');
  nameField.className = 'cfg-field';
  nameField.innerHTML =
    '<span class="cfg-label">名称</span><span class="cfg-control">' +
    '<input class="cfg-input" data-role="name" type="text" placeholder="例如 兔小贝" spellcheck="false"></span>';

  const kindRow = document.createElement('div');
  kindRow.className = 'cfg-kind-row';
  kindRow.innerHTML = [
    '<div class="cfg-segment">',
    '<label class="cfg-segment-item"><input type="radio" data-role="kind" value="drpy"><span>drpy-node</span></label>',
    '<label class="cfg-segment-item"><input type="radio" data-role="kind" value="js"><span>JS 规则文件</span></label>',
    '</div>',
  ].join('');

  const drpyGroup = document.createElement('div');
  drpyGroup.className = 'cfg-group';
  drpyGroup.dataset.role = 'drpy-fields';
  drpyGroup.innerHTML = [
    '<label class="cfg-field"><span class="cfg-label">服务端</span><span class="cfg-control">',
    '<input class="cfg-input" data-role="base_url" type="text" placeholder="http://192.168.0.201:5757" spellcheck="false"></span></label>',
    '<label class="cfg-field"><span class="cfg-label">源名称</span><span class="cfg-control">',
    '<input class="cfg-input" data-role="source_name" type="text" placeholder="兔小贝[儿]" spellcheck="false"></span></label>',
    '<label class="cfg-field"><span class="cfg-label">API 密码</span><span class="cfg-control">',
    '<input class="cfg-input" data-role="pwd" type="password" placeholder="dzyyds"></span></label>',
  ].join('');

  const jsGroup = document.createElement('div');
  jsGroup.className = 'cfg-js-group';
  jsGroup.dataset.role = 'js-fields';
  jsGroup.innerHTML = [
    '<div class="cfg-js-file">',
    '<button class="cfg-upload-btn" type="button" data-role="upload">选择 JS 文件</button>',
    '<span class="cfg-js-file-name" data-role="filename">未选择文件</span>',
    '<input type="file" accept=".js,.txt" data-role="file" hidden>',
    '</div>',
    '<textarea class="cfg-script" data-role="script" rows="7" spellcheck="false" ',
    'placeholder="也可以直接把 drpy 规则文件的内容粘贴到这里"></textarea>',
  ].join('');

  const actions = document.createElement('div');
  actions.className = 'cfg-source-actions';
  actions.innerHTML = [
    '<button class="cfg-test-btn" type="button" data-role="test">检测分类</button>',
    '<span class="cfg-source-status" data-role="status"></span>',
  ].join('');

  const cats = document.createElement('div');
  cats.className = 'cfg-cats';
  cats.dataset.role = 'cats';

  card.append(head, nameField, kindRow, drpyGroup, jsGroup, actions, cats);

  const nameInput = card.querySelector('[data-role="name"]');
  nameInput.value = src.name;
  nameInput.addEventListener('input', () => {
    src.name = nameInput.value;
  });

  const enabledBtn = card.querySelector('[data-role="enabled"]');
  enabledBtn.classList.toggle('on', src.enabled);
  enabledBtn.setAttribute('aria-pressed', src.enabled ? 'true' : 'false');
  enabledBtn.addEventListener('click', () => {
    src.enabled = !src.enabled;
    enabledBtn.classList.toggle('on', src.enabled);
    enabledBtn.setAttribute('aria-pressed', src.enabled ? 'true' : 'false');
  });

  card.querySelector('[data-role="del"]').addEventListener('click', () => {
    clearTimeout(src.autoTimer);
    const index = sourceDrafts.indexOf(src);
    if (index >= 0) sourceDrafts.splice(index, 1);
    renderSources();
  });

  card.querySelectorAll('[data-role="kind"]').forEach((input) => {
    input.checked = input.value === src.kind;
    input.addEventListener('change', () => {
      if (!input.checked) return;
      collectSourceInputs(src, card);
      src.kind = input.value === 'js' ? 'js' : 'drpy';
      // 换类型后旧的分类不再适用，等重新检测
      src.catOptions = [];
      src.categories = [];
      src.status = '';
      src.statusKind = '';
      renderSources();
    });
  });

  const drpyFields = card.querySelector('[data-role="drpy-fields"]');
  const jsFields = card.querySelector('[data-role="js-fields"]');
  drpyFields.style.display = src.kind === 'drpy' ? '' : 'none';
  jsFields.style.display = src.kind === 'js' ? '' : 'none';

  if (src.kind === 'drpy') {
    const baseInput = card.querySelector('[data-role="base_url"]');
    baseInput.value = src.base_url;
    baseInput.addEventListener('input', () => {
      src.base_url = baseInput.value;
    });
    baseInput.addEventListener('change', () => scheduleAutoTest(src));
    const sourceNameInput = card.querySelector('[data-role="source_name"]');
    sourceNameInput.value = src.source_name;
    sourceNameInput.addEventListener('input', () => {
      src.source_name = sourceNameInput.value;
    });
    sourceNameInput.addEventListener('change', () => scheduleAutoTest(src));
    const pwdInput = card.querySelector('[data-role="pwd"]');
    pwdInput.value = src.pwd;
    pwdInput.addEventListener('input', () => {
      src.pwd = pwdInput.value;
    });
    pwdInput.addEventListener('change', () => scheduleAutoTest(src));
  } else {
    const scriptInput = card.querySelector('[data-role="script"]');
    scriptInput.value = src.script;
    scriptInput.addEventListener('input', () => {
      src.script = scriptInput.value;
    });
    // 粘贴/输入结束（textarea 失焦）时自动检测，不用再点按钮
    scriptInput.addEventListener('change', () => scheduleAutoTest(src));

    const filename = card.querySelector('[data-role="filename"]');
    filename.textContent = src.script_name || '未选择文件';

    const fileInput = card.querySelector('[data-role="file"]');
    card.querySelector('[data-role="upload"]').addEventListener('click', () => fileInput.click());
    fileInput.addEventListener('change', () => {
      const file = fileInput.files && fileInput.files[0];
      if (!file) return;
      const reader = new FileReader();
      reader.onload = () => {
        src.script = String(reader.result || '');
        src.script_name = file.name;
        scriptInput.value = src.script;
        filename.textContent = file.name;
        scheduleAutoTest(src);
      };
      reader.onerror = () => sourceStatus(src, '读取文件失败，请重试', 'error');
      reader.readAsText(file);
    });
  }

  card.querySelector('[data-role="test"]').addEventListener('click', () => {
    // 手动点按钮时取消待触发的自动检测，避免重复请求
    clearTimeout(src.autoTimer);
    testSourceDraft(src, card);
  });

  const statusEl = card.querySelector('[data-role="status"]');
  statusEl.textContent = src.status;
  statusEl.className = 'cfg-source-status' + (src.statusKind ? ' ' + src.statusKind : '');

  renderSourceCats(card, src);
}

function renderSourceCats(card, src) {
  const box = card.querySelector('[data-role="cats"]');
  if (!box) return;
  box.innerHTML = '';

  if (!src.catOptions.length) {
    const hint = document.createElement('span');
    hint.className = 'cfg-cats-hint';
    if (!src.statusKind && src.id && !src.catsLoaded) {
      hint.textContent = '正在加载分类…';
    } else if (src.statusKind === 'error') {
      hint.textContent = '连接失败，暂时没有分类';
    } else {
      hint.textContent = '填完连接信息会自动检测，也可以点「检测分类」';
    }
    box.appendChild(hint);
    return;
  }

  src.catOptions.forEach((cat) => {
    const label = document.createElement('label');
    label.className = 'cfg-cat';

    const input = document.createElement('input');
    input.type = 'checkbox';
    input.value = cat.id;
    input.checked = src.categories.includes(cat.id);
    input.addEventListener('change', () => {
      if (input.checked) {
        if (!src.categories.includes(cat.id)) src.categories.push(cat.id);
      } else {
        src.categories = src.categories.filter((id) => id !== cat.id);
      }
    });

    const text = document.createElement('span');
    text.textContent = cat.name;

    label.append(input, text);
    box.appendChild(label);
  });
}

async function postJSON(path, payload) {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  const body = await res.json();
  if (body.code !== 0) throw new Error(body.msg || '请求失败');
  return body.data;
}

// 填完一项连接信息后自动检测：轻量防抖，避免边输入边请求上游。
// 必填项不完整时什么都不做，等用户填完最后一项再触发。
function scheduleAutoTest(src) {
  clearTimeout(src.autoTimer);
  src.autoTimer = setTimeout(() => {
    if (src.testing) return;
    if (src.kind === 'drpy' && (!src.base_url.trim() || !src.source_name.trim())) return;
    if (src.kind === 'js' && src.script.trim().length < 10) return;
    testSourceDraft(src, cfgSourcesEl.querySelector('[data-uid="' + src.uid + '"]'));
  }, 600);
}

// 用还没保存的表单内容连接一次，成功返回这个源自己的分类
async function testSourceDraft(src, card) {
  if (!card || src.testing) return;
  collectSourceInputs(src, card);

  if (src.kind === 'drpy' && (!src.base_url.trim() || !src.source_name.trim())) {
    sourceStatus(src, '请先填写服务端地址和源名称', 'error');
    return;
  }
  if (src.kind === 'js' && !src.script.trim()) {
    sourceStatus(src, '请先粘贴或上传 JS 规则文件', 'error');
    return;
  }

  src.testing = true;
  const btn = card.querySelector('[data-role="test"]');
  btn.disabled = true;
  btn.textContent = '检测中…';
  sourceStatus(src, '正在连接…', '');

  try {
    const cats = normalizeCategories(await postJSON('/api/source/test', sourcePayload(src)));
    src.catOptions = cats;
    // 只保留这次检测里仍然存在的勾选，避免把已经不存在的栏目 ID 存下去
    src.categories = src.categories.filter((id) => cats.some((cat) => cat.id === id));
    sourceStatus(
      src,
      cats.length ? '连接成功，共 ' + cats.length + ' 个分类' : '连接成功，但这个源没有返回分类',
      'ok'
    );
  } catch (err) {
    src.catOptions = [];
    // 保留已勾选的栏目 ID：配置改对后重新检测，勾选会自动恢复
    sourceStatus(src, err.message || '连接失败', 'error');
  } finally {
    src.testing = false;
    src.catsLoaded = true;
    btn.disabled = false;
    btn.textContent = '检测分类';
  }

  const live = cfgSourcesEl.querySelector('[data-uid="' + src.uid + '"]');
  if (live) renderSourceCats(live, src);
}

// 已保存的源：打开配置页时后台拉一次分类，让栏目名直接显示出来
async function loadSavedCategories(src) {
  try {
    const cats = normalizeCategories(await api('/api/categories?source_id=' + encodeURIComponent(src.id)));
    src.catOptions = cats;
    // 分类加载成功但不打断已有勾选：已保存 ID 不在列表里时保留，保存后仍然生效
    sourceStatus(src, '已加载 ' + cats.length + ' 个分类', '');
  } catch (err) {
    src.catOptions = [];
    sourceStatus(src, '分类加载失败：' + err.message, 'error');
  }
  src.catsLoaded = true;
  const card = cfgSourcesEl.querySelector('[data-uid="' + src.uid + '"]');
  if (card) renderSourceCats(card, src);
}

// 当前配置回显到表单；兼容旧版扁平结构（protocol/host 直接在顶层）
function applyConfigForm(data) {
  const alist = data.alist || data || {};

  cfgProtocol.value = String(alist.protocol || '').toLowerCase() === 'https' ? 'https' : 'http';
  cfgAddress.value = alist.host || '';
  cfgPort.value = alist.port || '';
  cfgUsername.value = alist.username || '';
  cfgPassword.value = alist.password || '';
  cfgVideoPath.value = alist.base_path || '';
  setAlistEnabled(!alist.disabled);

  cfgPassword.type = 'password';
  cfgEye.innerHTML = ICONS.eyeOff;
  cfgEye.setAttribute('aria-label', '显示密码');

  sourceDrafts.length = 0;
  const list = Array.isArray(data.sources) ? data.sources : [];
  list.forEach((raw) => sourceDrafts.push(newSourceDraft(raw)));
  renderSources();

  // 已保存且启用的源：打开页面就能看到自己的分类，不用重新点检测
  sourceDrafts.forEach((src) => {
    if (src.id && src.enabled) loadSavedCategories(src);
  });
}

// 设置页「服务器配置」右侧的小字：优先展示当前默认源
// 设置抽屉「数据源配置」右侧的小字：在线源数量 + 网盘是否参与
function setServerSummary(settings) {
  if (!settings) {
    serverSub.textContent = '未配置';
    return;
  }
  const sources = (Array.isArray(settings.sources) ? settings.sources : []).filter((s) => s.enabled);
  const alist = settings.alist || settings;
  const parts = [];
  if (sources.length) parts.push('在线源 × ' + sources.length);
  if (alist.host && !alist.disabled) parts.push('网盘');
  serverSub.textContent = parts.length ? parts.join(' · ') : '未配置';
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

// 「设置」→「数据源配置」，逐级返回
async function openConfigPage() {
  hideConfigError();
  closeDrawer();
  pageConfig.hidden = false;
  configOpen = true;
  try {
    // 已保存源的分区在 applyConfigForm 里异步加载，不阻塞页面打开
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
    setServerSummary(data);
  } catch (err) {
    serverSub.textContent = '未配置';
  }
}

// 密码框右侧的小眼睛：点一下在明文/密文之间切换
function bindPasswordEye(button, input) {
  button.addEventListener('click', () => {
    const showing = input.type === 'text';
    input.type = showing ? 'password' : 'text';
    button.innerHTML = showing ? ICONS.eyeOff : ICONS.eye;
    button.setAttribute('aria-label', showing ? '显示密码' : '隐藏密码');
  });
}

bindPasswordEye(cfgEye, cfgPassword);

// 网盘开关：关掉后网盘不参与首页混排，在线源不受影响
cfgAlistToggle.addEventListener('click', () => setAlistEnabled(!isAlistEnabled()));

// 添加一个空白在线源，填完信息点「检测分类」就会列出它自己的分类
cfgAddSource.addEventListener('click', () => {
  sourceDrafts.push(newSourceDraft({}));
  renderSources();
  const last = cfgSourcesEl.lastElementChild;
  if (last) last.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
});

cfgForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  hideConfigError();

  // 保存前从 DOM 同步一遍，防止浏览器自动填充等没有触发 input 的情况
  sourceDrafts.forEach((src) => {
    const card = cfgSourcesEl.querySelector('[data-uid="' + src.uid + '"]');
    if (card) collectSourceInputs(src, card);
  });

  // 网盘：填了地址就必须有合法端口；完全不填则允许保存（只用在在线源）
  const alistHost = cfgAddress.value.trim();
  const alistPort = cfgPort.value.trim();
  if (alistHost) {
    if (!alistPort || !/^\d+$/.test(alistPort)) {
      showConfigError('填写了网盘地址时，请填写数字端口');
      return;
    }
  }

  // 启用的源必须配置完整；没启用的源允许先存着，之后随时打开
  for (const src of sourceDrafts) {
    if (!src.enabled) continue;
    const label = src.name.trim() || '未命名数据源';
    if (src.kind === 'drpy' && !src.base_url.trim()) {
      showConfigError('数据源「' + label + '」还没有填写服务端地址');
      return;
    }
    if (src.kind === 'drpy' && !src.source_name.trim()) {
      showConfigError('数据源「' + label + '」还没有填写源名称');
      return;
    }
    if (src.kind === 'js' && !src.script.trim()) {
      showConfigError('数据源「' + label + '」还没有上传或粘贴 JS 规则文件');
      return;
    }
  }

  const payload = {
    alist: {
      protocol: cfgProtocol.value,
      host: alistHost,
      port: alistPort,
      username: cfgUsername.value.trim(),
      password: cfgPassword.value,
      base_path: cfgVideoPath.value.trim(),
      disabled: !isAlistEnabled(),
    },
    sources: sourceDrafts.map(sourcePayload),
  };

  cfgSave.disabled = true;
  cfgSave.textContent = '保存中…';
  try {
    const data = await postJSON('/api/config', payload);
    setServerSummary(data || payload);
    // 数据源变化后要重新拉一次首页，最简单可靠的方式是整页重载
    showToast('保存成功，首页已按新数据源生效');
    setTimeout(() => location.reload(), 900);
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
muteBtn.addEventListener('click', () => setSound(!soundOn));

// 观看历史 / 喜欢：切换 Tab 后读对应的 localStorage 重绘网格
meTabs.forEach((tab) =>
  tab.addEventListener('click', () => {
    meTab = tab.dataset.meTab === 'likes' ? 'likes' : 'history';
    renderMineTab();
  })
);

// 清空按钮：只清当前 Tab 的数据，喜欢清空后屏幕上的红心也要同步变回空心
meClear.addEventListener('click', () => {
  if (meTab === 'likes') {
    likedVideos = [];
    likedPaths.clear();
    writeLocalList(LIKED_KEY, []);
    feedEl.querySelectorAll('.slide').forEach((slide) => setLikeUI(slide, false));
    showToast('已清空喜欢');
  } else {
    writeLocalList(HISTORY_KEY, []);
    showToast('已清空观看历史');
  }
  renderMineTab();
});

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

  // 在线源的 path 是「src:源ID:条目ID」，里面混着封面地址的 /，
  // 不能按季/文件名排序，保持栏目接口返回的顺序即可（就是最新更新的顺序）。
  const online = items.some((item) => item.source_id || item.category_id);
  if (!online) {
    entries.sort((a, b) => {
      const ao = a.season ? a.season.order : Number.MAX_SAFE_INTEGER;
      const bo = b.season ? b.season.order : Number.MAX_SAFE_INTEGER;
      if (ao !== bo) return ao - bo;
      const ap = a.item.path.toLowerCase();
      const bp = b.item.path.toLowerCase();
      return ap < bp ? -1 : ap > bp ? 1 : 0;
    });
  }

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
  // 视频目录直接取服务器配置里的 AList BasePath（URL 上的 path 仅用于临时调试）
  if (!params.get('path')) {
    try {
      const cfg = await api('/api/config');
      const alist = cfg.alist || cfg;
      if (alist.base_path) currentDir = alist.base_path;
    } catch (err) {
      // 配置接口不可用时继续用内置默认目录
    }
  }

  try {
    const data = await api('/api/feed?path=' + encodeURIComponent(currentDir));
    if (!Array.isArray(data) || data.length === 0) {
      meCount.textContent = '0';
      meCollections.textContent = '0';
      showEmpty('该目录下没有视频文件');
      return;
    }

    // 按视频所属合集分组：一个文件夹 = 一个合集（根目录「启蒙」下会有多个合集）。
    // 但文件夹名是「第 1 季」这类分季目录时，合集往上取一层，同一部的各季合并。
    const groups = new Map();
    data.forEach((item) => {
      // 在线源按「源 + 栏目」合成合集，避免兔小贝和贝乐虎的同名栏目串在一起
      if (item.source_id || item.category_id) {
        const key = 'cat:' + (item.source_id || '') + ':' + (item.category_id || item.category);
        if (!groups.has(key)) {
          groups.set(key, {
            name: item.category || '在线内容',
            // 记住源 ID + 栏目 ID：合集面板要按它去拉该栏目的全部视频
            sourceId: item.source_id || '',
            categoryId: item.category_id || '',
            items: [],
          });
        }
        groups.get(key).items.push(item);
        return;
      }
      // 网盘条目的 path 是目录层级，按所属文件夹归合集
      const dirPath = item.path.indexOf('/') >= 0
        ? item.path.slice(0, item.path.lastIndexOf('/'))
        : '/';
      const { key, name } = collectionDirOf(dirPath);
      if (!groups.has(key)) groups.set(key, { name, items: [] });
      groups.get(key).items.push(item);
    });

    let colIndex = 0;
    groups.forEach((group) => {
      const col = {
        id: 'col-' + colIndex,
        name: group.name,
        // 在线栏目才有 categoryId；网盘合集为空，面板仍按已载入的集数展示
        categoryId: group.categoryId || '',
        sourceId: group.sourceId || '',
        episodes: [],
      };
      // 同一部的各季合并成一个合集：按季排好序，并给每集标上所属季与季内序号
      const built = buildEpisodes(group.items, col.id);
      col.episodes = built.episodes;
      col.seasonTotal = built.seasonTotal;
      // 首页只加载了每个栏目的前几条，真实总集数来自后端 total_count，
      // 不能拿 group.items.length 当总数（否则会显示「共 4 集」而实际是 12 集）
      col.totalCount = group.items.reduce((max, item) => Math.max(max, item.total_count || 0), 0);
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
