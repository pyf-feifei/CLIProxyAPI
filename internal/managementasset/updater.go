package managementasset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"
)

const (
	defaultManagementReleaseURL   = "https://api.github.com/repos/router-for-me/Cli-Proxy-API-Management-Center/releases/latest"
	defaultManagementFallbackURL  = "https://cpamc.router-for.me/"
	managementAssetName           = "management.html"
	httpUserAgent                 = "CLIProxyAPI-management-updater"
	managementSyncMinInterval     = 30 * time.Second
	updateCheckInterval           = 3 * time.Hour
	managementSessionPatchMarker  = "<!-- cliproxyapi-session-refresh-patch -->"
	managementKeyUsagePanelMarker = "<!-- cliproxyapi-key-usage-panel-patch -->"
	maxAssetDownloadSize          = 50 << 20 // 10 MB safety limit for management asset downloads
)

const managementRouteFlashGuardScript = `<script>(function(){if(window.__cliproxyRouteFlashGuardV1){return;}window.__cliproxyRouteFlashGuardV1=true;var SESSION_KEY='cli-proxy-session-auth';var currentHash=window.location.hash||'';if(!currentHash||currentHash.indexOf('#/login')===0){return;}var hasSession=false;try{hasSession=!!window.sessionStorage.getItem(SESSION_KEY);}catch(_err){hasSession=false;}if(!hasSession){return;}var STYLE_ID='cliproxy-refresh-guard-style';function ensureStyle(){if(document.getElementById(STYLE_ID)){return;}var style=document.createElement('style');style.id=STYLE_ID;style.textContent='html[data-cliproxy-refresh-guard=\"on\"] #root{visibility:hidden !important;}';document.head.appendChild(style);}function clearGuard(){document.documentElement.removeAttribute('data-cliproxy-refresh-guard');window.removeEventListener('hashchange',onHashChange,true);if(interval){window.clearInterval(interval);interval=null;}}function isLoginHash(){return(window.location.hash||'').indexOf('#/login')===0;}function onHashChange(){if(!isLoginHash()){clearGuard();}}ensureStyle();document.documentElement.setAttribute('data-cliproxy-refresh-guard','on');window.addEventListener('hashchange',onHashChange,true);var interval=window.setInterval(function(){if(!isLoginHash()){clearGuard();}},100);window.setTimeout(clearGuard,4000);window.addEventListener('pageshow',function(){if(!isLoginHash()){clearGuard();}},{once:true});})();</script>`

const managementKeyUsagePanelPatch = managementKeyUsagePanelMarker + `<style>.cpa-key-usage-trigger{position:fixed;right:22px;bottom:22px;z-index:2147483000;border:0;border-radius:999px;padding:12px 16px;background:linear-gradient(135deg,#2563eb,#7c3aed);color:#fff;font-weight:700;box-shadow:0 12px 32px rgba(37,99,235,.32);cursor:pointer}.cpa-key-usage-panel{position:fixed;right:22px;bottom:78px;z-index:2147483000;width:min(920px,calc(100vw - 32px));max-height:min(760px,calc(100vh - 104px));display:none;flex-direction:column;border:1px solid rgba(148,163,184,.35);border-radius:18px;background:var(--bg-primary,#fff);color:var(--text-primary,#0f172a);box-shadow:0 24px 70px rgba(15,23,42,.28);overflow:hidden}.cpa-key-usage-panel.cpa-key-usage-open{display:flex}.cpa-key-usage-head{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:16px 18px;border-bottom:1px solid var(--border-color,rgba(148,163,184,.25));background:linear-gradient(135deg,rgba(37,99,235,.10),rgba(124,58,237,.10))}.cpa-key-usage-title{display:flex;flex-direction:column;gap:3px}.cpa-key-usage-title strong{font-size:17px}.cpa-key-usage-title span{font-size:12px;color:var(--text-secondary,#64748b)}.cpa-key-usage-actions{display:flex;align-items:center;gap:8px}.cpa-key-usage-btn{border:1px solid var(--border-color,rgba(148,163,184,.35));border-radius:10px;padding:8px 11px;background:var(--bg-secondary,#f8fafc);color:var(--text-primary,#0f172a);cursor:pointer}.cpa-key-usage-body{padding:16px 18px;overflow:auto}.cpa-key-usage-toolbar{display:grid;grid-template-columns:minmax(0,1fr) auto auto;gap:10px;align-items:center;margin-bottom:14px}.cpa-key-usage-input{border:1px solid var(--border-color,rgba(148,163,184,.35));border-radius:10px;padding:9px 11px;background:var(--bg-primary,#fff);color:var(--text-primary,#0f172a);min-width:0}.cpa-key-usage-check{display:flex;align-items:center;gap:6px;font-size:13px;color:var(--text-secondary,#64748b);white-space:nowrap}.cpa-key-usage-summary{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:10px;margin-bottom:14px}.cpa-key-usage-card{border:1px solid var(--border-color,rgba(148,163,184,.25));border-radius:14px;padding:12px;background:var(--bg-secondary,#f8fafc)}.cpa-key-usage-card span{display:block;font-size:12px;color:var(--text-secondary,#64748b);margin-bottom:4px}.cpa-key-usage-card strong{font-size:21px}.cpa-key-usage-table{width:100%;border-collapse:collapse;font-size:13px}.cpa-key-usage-table th,.cpa-key-usage-table td{padding:10px 8px;border-bottom:1px solid var(--border-color,rgba(148,163,184,.22));text-align:left;vertical-align:middle}.cpa-key-usage-table th{font-size:12px;color:var(--text-secondary,#64748b);font-weight:700;background:var(--bg-secondary,#f8fafc);position:sticky;top:0}.cpa-key-usage-key{font-family:Consolas,Monaco,Courier New,monospace;white-space:nowrap}.cpa-key-usage-url{max-width:220px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--text-secondary,#64748b)}.cpa-key-usage-bars{display:flex;align-items:flex-end;gap:2px;height:28px;min-width:130px}.cpa-key-usage-bar{width:5px;min-height:2px;border-radius:4px 4px 0 0;background:#22c55e}.cpa-key-usage-bar-fail{background:#ef4444}.cpa-key-usage-empty{padding:24px;border:1px dashed var(--border-color,rgba(148,163,184,.35));border-radius:14px;text-align:center;color:var(--text-secondary,#64748b)}.cpa-key-usage-status-error{color:#dc2626!important}.cpa-key-usage-auth{display:none;grid-template-columns:minmax(0,1fr) minmax(0,1fr) auto;gap:8px;margin-bottom:12px;padding:10px;border:1px solid rgba(245,158,11,.35);border-radius:12px;background:rgba(245,158,11,.08)}.cpa-key-usage-auth.cpa-key-usage-show{display:grid}.cpa-key-usage-auth small{grid-column:1/-1;color:var(--text-secondary,#64748b)}@media(max-width:720px){.cpa-key-usage-panel{right:12px;bottom:70px;width:calc(100vw - 24px)}.cpa-key-usage-toolbar,.cpa-key-usage-auth{grid-template-columns:1fr}.cpa-key-usage-summary{grid-template-columns:repeat(2,minmax(0,1fr))}.cpa-key-usage-table{min-width:760px}.cpa-key-usage-trigger{right:14px;bottom:14px}}</style><script>(function(){if(window.__cliproxyKeyUsagePanelV1){return;}window.__cliproxyKeyUsagePanelV1=true;var E='enc::v1::',S='cli-proxy-api-webui::secure-storage',P='cli-proxy-auth',K='cli-proxy-session-auth',tempKey='',timer=null,lastRows=[];function q(s,r){return(r||document).querySelector(s)}function esc(v){return String(v==null?'':v).replace(/[&<>\"]/g,function(c){return{'&':'&amp;','<':'&lt;','>':'&gt;','\"':'&quot;'}[c]})}function enc(t){return new TextEncoder().encode(t)}function dec(b){return new TextDecoder().decode(b)}function kb(){try{return enc(S+'|'+location.host+'|'+navigator.userAgent)}catch(e){return enc(S)}}function xb(d,k){var o=new Uint8Array(d.length);for(var i=0;i<d.length;i++){o[i]=d[i]^k[i%k.length]}return o}function b64(s){var r=atob(s),o=new Uint8Array(r.length);for(var i=0;i<r.length;i++){o[i]=r.charCodeAt(i)}return o}function de(v){if(!v||v.indexOf(E)!==0){return v}try{return dec(xb(b64(v.slice(E.length)),kb()))}catch(e){return v}}function json(st,n){try{var r=st.getItem(n);return r==null?null:JSON.parse(de(r))}catch(e){return null}}function str(st,n){try{var r=st.getItem(n);if(r==null){return''}r=de(r);try{return String(JSON.parse(r)||'')}catch(e){return String(r||'')}}catch(e){return''}}function base(v){var s=String(v||'').trim();if(!s){return location.protocol+'//'+location.host}s=s.replace(/\/?v0\/management\/?$/i,'').replace(/\/+$/g,'');if(!/^https?:\/\//i.test(s)){s='http://'+s}return s}function auth(){var se=json(sessionStorage,K)||{},pe=json(localStorage,P)||{},st=pe.state||pe||{},fb=localStorage.getItem('cpa-key-usage-temp-base')||'';return{apiBase:base(se.apiBase||st.apiBase||st.apiUrl||str(localStorage,'apiBase')||str(localStorage,'apiUrl')||fb),managementKey:String(se.managementKey||st.managementKey||str(localStorage,'managementKey')||tempKey||'').trim()}}function mask(v){var s=String(v||'');if(!s){return'-'}return s.length<=10?s.slice(0,3)+'…'+s.slice(-2):s.slice(0,6)+'…'+s.slice(-4)}function split(v){var s=String(v),i=s.indexOf('|');return i<0?{baseUrl:'',apiKey:s}:{baseUrl:s.slice(0,i),apiKey:s.slice(i+1)}}function flat(d){var rows=[];Object.keys(d||{}).forEach(function(p){var items=d[p]||{};Object.keys(items).forEach(function(c){var e=items[c]||{},sp=split(c),ok=Number(e.success||0),bad=Number(e.failed||0);rows.push({provider:p,baseUrl:sp.baseUrl,apiKey:sp.apiKey,success:ok,failed:bad,total:ok+bad,recent:Array.isArray(e.recent_requests)?e.recent_requests:[]})})});rows.sort(function(a,b){return b.total-a.total||a.provider.localeCompare(b.provider)});return rows}function summary(rows){return rows.reduce(function(a,r){a.total+=r.total;a.success+=r.success;a.failed+=r.failed;a.providers[r.provider]=1;return a},{total:0,success:0,failed:0,providers:{}})}function rate(r){return r.total?(Math.round(r.success*1000/r.total)/10)+'%':'-'}function bars(r){var bs=r.recent.slice(-20),m=1;bs.forEach(function(b){m=Math.max(m,Number(b.success||0)+Number(b.failed||0))});if(!bs.length){return'-'}return bs.map(function(b){var ok=Number(b.success||0),bad=Number(b.failed||0),h=Math.max(2,Math.round((ok+bad)*28/m)),cl=bad>ok?'cpa-key-usage-bar cpa-key-usage-bar-fail':'cpa-key-usage-bar';return'<span class="'+cl+'" style="height:'+h+'px" title="'+esc(b.time)+' success '+ok+', failed '+bad+'"></span>'}).join('')}function render(){var search=q('#cpa-key-usage-search'),f=search?search.value.trim().toLowerCase():'',rows=lastRows.filter(function(r){return!f||r.provider.toLowerCase().indexOf(f)>=0||r.baseUrl.toLowerCase().indexOf(f)>=0||r.apiKey.toLowerCase().indexOf(f)>=0}),s=summary(rows),sum=q('#cpa-key-usage-summary'),body=q('#cpa-key-usage-table-body');if(sum){sum.innerHTML='<div class="cpa-key-usage-card"><span>总计</span><strong>'+s.total+'</strong></div><div class="cpa-key-usage-card"><span>成功</span><strong>'+s.success+'</strong></div><div class="cpa-key-usage-card"><span>失败</span><strong>'+s.failed+'</strong></div><div class="cpa-key-usage-card"><span>提供方s</span><strong>'+Object.keys(s.providers).length+'</strong></div>'}if(!body){return}if(!rows.length){body.innerHTML='<tr><td colspan="8"><div class="cpa-key-usage-empty">暂无 Key 使用数据。</div></td></tr>';return}body.innerHTML=rows.map(function(r){return'<tr><td>'+esc(r.provider)+'</td><td class="cpa-key-usage-key">'+esc(mask(r.apiKey))+'</td><td class="cpa-key-usage-url" title="'+esc(r.baseUrl||'-')+'">'+esc(r.baseUrl||'-')+'</td><td>'+r.total+'</td><td>'+r.success+'</td><td>'+r.failed+'</td><td>'+rate(r)+'</td><td><div class="cpa-key-usage-bars">'+bars(r)+'</div></td></tr>'}).join('')}function status(m,e){var el=q('#cpa-key-usage-status');if(!el){return}el.textContent=m||'';el.className=e?'cpa-key-usage-status-error':''}function load(){var a=auth(),h={Accept:'application/json'};if(a.managementKey){h.Authorization='Bearer '+a.managementKey}var box=q('#cpa-key-usage-auth');if(box){box.classList.toggle('cpa-key-usage-show',!a.managementKey)}status('加载中...',false);fetch(a.apiBase+'/v0/management/api-key-usage',{headers:h,cache:'no-store'}).then(function(r){if(!r.ok){throw new Error('HTTP '+r.status)}return r.json()}).then(function(d){lastRows=flat(d);render();status('已更新 '+new Date().toLocaleTimeString(),false)}).catch(function(e){status('失败 to load API key usage: '+(e&&e.message?e.message:e),true)})}function stop(){if(timer){clearInterval(timer);timer=null}}function restart(){stop();var p=q('#cpa-key-usage-panel'),a=q('#cpa-key-usage-auto');if(p&&p.classList.contains('cpa-key-usage-open')&&a&&a.checked){timer=setInterval(load,10000)}}function ensure(){if(q('#cpa-key-usage-panel')){return}var p=document.createElement('div');p.id='cpa-key-usage-panel';p.className='cpa-key-usage-panel';p.innerHTML='<div class="cpa-key-usage-head"><div class="cpa-key-usage-title"><strong>Key 使用监控</strong><span id="cpa-key-usage-status">就绪</span></div><div class="cpa-key-usage-actions"><button class="cpa-key-usage-btn" id="cpa-key-usage-refresh" type="button">刷新</button><button class="cpa-key-usage-btn" id="cpa-key-usage-close" type="button">×</button></div></div><div class="cpa-key-usage-body"><div id="cpa-key-usage-auth" class="cpa-key-usage-auth"><input class="cpa-key-usage-input" id="cpa-key-usage-base" placeholder="API 地址，默认当前站点"><input class="cpa-key-usage-input" id="cpa-key-usage-key" type="password" placeholder="管理密钥，仅本面板临时使用"><button class="cpa-key-usage-btn" id="cpa-key-usage-apply-auth" type="button">Apply</button><small>优先读取当前登录的管理密钥；这里填写的密钥只在本页面会话临时使用。</small></div><div class="cpa-key-usage-toolbar"><input class="cpa-key-usage-input" id="cpa-key-usage-search" placeholder="搜索提供方、Base URL 或账号/Key"><label class="cpa-key-usage-check"><input type="checkbox" id="cpa-key-usage-auto" checked>10 秒自动刷新</label><button class="cpa-key-usage-btn" id="cpa-key-usage-clear" type="button">清空筛选</button></div><div id="cpa-key-usage-summary" class="cpa-key-usage-summary"></div><table class="cpa-key-usage-table"><thead><tr><th>提供方</th><th>账号/Key</th><th>Base URL</th><th>总计</th><th>成功</th><th>失败</th><th>成功率</th><th>最近请求</th></tr></thead><tbody id="cpa-key-usage-table-body"></tbody></table></div>';var t=document.createElement('button');t.id='cpa-key-usage-trigger';t.className='cpa-key-usage-trigger';t.type='button';t.textContent='Key 使用';document.body.appendChild(p);document.body.appendChild(t);function open(){p.classList.add('cpa-key-usage-open');load();restart()}function close(){p.classList.remove('cpa-key-usage-open');stop()}t.addEventListener('click',function(){p.classList.contains('cpa-key-usage-open')?close():open()});q('#cpa-key-usage-close').addEventListener('click',close);q('#cpa-key-usage-refresh').addEventListener('click',load);q('#cpa-key-usage-search').addEventListener('input',render);q('#cpa-key-usage-clear').addEventListener('click',function(){q('#cpa-key-usage-search').value='';render()});q('#cpa-key-usage-auto').addEventListener('change',restart);q('#cpa-key-usage-apply-auth').addEventListener('click',function(){var b=q('#cpa-key-usage-base').value.trim();tempKey=q('#cpa-key-usage-key').value.trim();if(b){localStorage.setItem('cpa-key-usage-temp-base',b)}load()});var saved=localStorage.getItem('cpa-key-usage-temp-base');if(saved){q('#cpa-key-usage-base').value=saved}render()}if(document.readyState==='loading'){document.addEventListener('DOMContentLoaded',ensure)}else{ensure()}})();</script>`

const managementSessionPatchScript = `<script>
(function () {
  if (window.__cliproxySessionPatchV1) {
    return;
  }
  window.__cliproxySessionPatchV1 = true;

  var SESSION_KEY = 'cli-proxy-session-auth';
  var ENC_PREFIX = 'enc::v1::';
  var SECRET_SALT = 'cli-proxy-api-webui::secure-storage';
  var recentLoginCaptureAt = 0;
  var recentUserInteractionAt = 0;

  function encodeText(text) {
    return new TextEncoder().encode(text);
  }

  function decodeText(bytes) {
    return new TextDecoder().decode(bytes);
  }

  function getKeyBytes() {
    try {
      return encodeText(SECRET_SALT + '|' + window.location.host + '|' + navigator.userAgent);
    } catch (_err) {
      return encodeText(SECRET_SALT);
    }
  }

  function xorBytes(data, keyBytes) {
    var result = new Uint8Array(data.length);
    for (var i = 0; i < data.length; i++) {
      result[i] = data[i] ^ keyBytes[i % keyBytes.length];
    }
    return result;
  }

  function toBase64(bytes) {
    var binary = '';
    for (var i = 0; i < bytes.length; i++) {
      binary += String.fromCharCode(bytes[i]);
    }
    return btoa(binary);
  }

  function fromBase64(base64) {
    var binary = atob(base64);
    var bytes = new Uint8Array(binary.length);
    for (var i = 0; i < binary.length; i++) {
      bytes[i] = binary.charCodeAt(i);
    }
    return bytes;
  }

  function encryptData(value) {
    if (!value) {
      return value;
    }
    try {
      return ENC_PREFIX + toBase64(xorBytes(encodeText(value), getKeyBytes()));
    } catch (_err) {
      return value;
    }
  }

  function decryptData(payload) {
    if (!payload || payload.indexOf(ENC_PREFIX) !== 0) {
      return payload;
    }
    try {
      return decodeText(xorBytes(fromBase64(payload.slice(ENC_PREFIX.length)), getKeyBytes()));
    } catch (_err) {
      return payload;
    }
  }

  function encodeStoredValue(value) {
    return encryptData(JSON.stringify(value));
  }

  function decodeStoredValue(raw) {
    var payload = raw;
    if (payload && payload.indexOf(ENC_PREFIX) === 0) {
      payload = decryptData(payload);
    }
    return JSON.parse(payload);
  }

  function readSession() {
    try {
      var raw = window.sessionStorage.getItem(SESSION_KEY);
      if (!raw) {
        return null;
      }
      var parsed = JSON.parse(raw);
      if (!parsed || typeof parsed.apiBase !== 'string' || typeof parsed.managementKey !== 'string') {
        return null;
      }
      var normalizedApiBase = normalizeApiBaseValue(parsed.apiBase);
      return {
        apiBase: normalizedApiBase || parsed.apiBase,
        managementKey: parsed.managementKey,
        sessionOnly: parsed.sessionOnly === true,
        updatedAt: Number(parsed.updatedAt || 0)
      };
    } catch (_err) {
      return null;
    }
  }

  function writeSession(apiBase, managementKey, sessionOnly) {
    apiBase = normalizeApiBaseValue(apiBase);
    managementKey = String(managementKey || '').trim();
    if (!apiBase || !managementKey) {
      return;
    }
    try {
      window.sessionStorage.setItem(SESSION_KEY, JSON.stringify({
        apiBase: apiBase,
        managementKey: managementKey,
        sessionOnly: sessionOnly === true,
        updatedAt: Date.now()
      }));
    } catch (_err) {}
  }

  function updateSessionMode(sessionOnly) {
    var current = readSession();
    if (!current) {
      return;
    }
    writeSession(current.apiBase, current.managementKey, sessionOnly);
  }

  function clearSession() {
    try {
      window.sessionStorage.removeItem(SESSION_KEY);
    } catch (_err) {}
  }

  function normalizeManagementPath(pathname) {
    pathname = String(pathname || '');
    pathname = pathname.replace(/\/management\.html(?=\/|$)/ig, '');
    pathname = pathname.replace(/\/v0\/management\/v0\/management(?=\/|$)/ig, '/v0/management');
    pathname = pathname.replace(/\/{2,}/g, '/');
    return pathname || '/';
  }

  function normalizeApiBaseValue(apiBase) {
    apiBase = String(apiBase || '').trim();
    if (!apiBase) {
      return '';
    }
    try {
      var parsedUrl = new URL(apiBase, window.location.href);
      var marker = '/v0/management';
      var pathname = normalizeManagementPath(parsedUrl.pathname);
      var lowerPath = pathname.toLowerCase();
      var idx = lowerPath.indexOf(marker);
      if (idx >= 0) {
        pathname = pathname.slice(0, idx);
      }
      pathname = pathname.replace(/\/+$/, '');
      return parsedUrl.origin + pathname;
    } catch (_err) {
      return apiBase;
    }
  }

  function normalizeApiBase(requestUrl) {
    return normalizeApiBaseValue(requestUrl);
  }

  function normalizeManagementRequestUrl(requestUrl) {
    if (!requestUrl) {
      return requestUrl;
    }
    try {
      var parsedUrl = new URL(requestUrl, window.location.href);
      parsedUrl.pathname = normalizeManagementPath(parsedUrl.pathname);
      return parsedUrl.toString();
    } catch (_err) {
      return requestUrl;
    }
  }

  function isManagementURL(requestUrl) {
    return String(requestUrl || '').toLowerCase().indexOf('/v0/management') >= 0;
  }

  function looksLikeManualLogin() {
    return (window.location.hash || '').indexOf('#/login') === 0 && (Date.now() - recentUserInteractionAt) < 5000;
  }

  function sanitizePersistedAuth(raw) {
    try {
      var parsed = decodeStoredValue(raw);
      if (!parsed || typeof parsed !== 'object' || !parsed.state || typeof parsed.state !== 'object') {
        return raw;
      }
      parsed.state.rememberPassword = false;
      delete parsed.state.managementKey;
      return encodeStoredValue(parsed);
    } catch (_err) {
      return raw;
    }
  }

  function shouldPersistAcrossBrowserSessions(raw) {
    try {
      var parsed = decodeStoredValue(raw);
      if (!parsed || typeof parsed !== 'object' || !parsed.state || typeof parsed.state !== 'object') {
        return false;
      }
      return parsed.state.rememberPassword === true;
    } catch (_err) {
      return false;
    }
  }

  function hydratePersistedAuth(raw, session) {
    var parsed = null;

    if (raw) {
      try {
        parsed = decodeStoredValue(raw);
      } catch (_err) {
        parsed = null;
      }
    }

    if (!parsed || typeof parsed !== 'object') {
      parsed = { state: {}, version: 0 };
    }
    if (!parsed.state || typeof parsed.state !== 'object') {
      parsed.state = {};
    }
    if (typeof parsed.version === 'undefined') {
      parsed.version = 0;
    }

    parsed.state.apiBase = session.apiBase;
    parsed.state.apiUrl = session.apiBase;
    parsed.state.rememberPassword = false;

    return encodeStoredValue(parsed);
  }

  function captureSession(requestUrl, authorization) {
    if (!isManagementURL(requestUrl)) {
      return;
    }

    var match = String(authorization || '').match(/^\s*Bearer\s+(.+?)\s*$/i);
    if (!match || !match[1]) {
      return;
    }

    var apiBase = normalizeApiBase(requestUrl);
    if (!apiBase) {
      return;
    }

    var current = readSession();
    writeSession(apiBase, match[1], current ? current.sessionOnly : true);

    if ((window.location.hash || '').indexOf('#/login') === 0) {
      recentLoginCaptureAt = Date.now();
    }
  }

  function markInteraction() {
    recentUserInteractionAt = Date.now();
  }

  window.addEventListener('pointerdown', markInteraction, true);
  window.addEventListener('keydown', markInteraction, true);
  window.addEventListener('submit', markInteraction, true);

  var originalGetItem = Storage.prototype.getItem;
  var originalSetItem = Storage.prototype.setItem;
  var originalRemoveItem = Storage.prototype.removeItem;

  Storage.prototype.getItem = function (key) {
    if (this === window.localStorage) {
      var session = readSession();
      if (session && session.sessionOnly) {
        if (key === 'cli-proxy-auth') {
          return hydratePersistedAuth(originalGetItem.call(this, key), session);
        }
        if (key === 'isLoggedIn') {
          return 'true';
        }
        if (key === 'apiBase' || key === 'apiUrl') {
          return encodeStoredValue(session.apiBase);
        }
        if (key === 'managementKey') {
          return encodeStoredValue(session.managementKey);
        }
      }
    }

    return originalGetItem.call(this, key);
  };

  Storage.prototype.setItem = function (key, value) {
    if (this === window.localStorage) {
      var session = readSession();

      if (key === 'cli-proxy-auth' && session && session.sessionOnly) {
        if (looksLikeManualLogin() && shouldPersistAcrossBrowserSessions(String(value))) {
          updateSessionMode(false);
          return originalSetItem.call(this, key, value);
        }
        return originalSetItem.call(this, key, sanitizePersistedAuth(String(value)));
      }

      if (key === 'isLoggedIn' && value === 'true') {
        if (session && session.sessionOnly) {
          return;
        }
        if (session) {
          updateSessionMode(false);
        }
      }
    }

    return originalSetItem.call(this, key, value);
  };

  Storage.prototype.removeItem = function (key) {
    if (this === window.localStorage && key === 'isLoggedIn') {
      var looksLikeFreshSessionOnlyLogin =
        (window.location.hash || '').indexOf('#/login') === 0 &&
        (Date.now() - recentLoginCaptureAt) < 3000;
      if (looksLikeFreshSessionOnlyLogin) {
        updateSessionMode(true);
      } else {
        clearSession();
      }
    }

    return originalRemoveItem.call(this, key);
  };

  window.addEventListener('unauthorized', clearSession);

  if (window.XMLHttpRequest) {
    var originalOpen = XMLHttpRequest.prototype.open;
    var originalSetRequestHeader = XMLHttpRequest.prototype.setRequestHeader;
    var originalSend = XMLHttpRequest.prototype.send;

    XMLHttpRequest.prototype.open = function (method, url) {
      var normalizedUrl = normalizeManagementRequestUrl(url);
      this.__cliproxyPatchUrl = normalizedUrl;
      this.__cliproxyPatchAuth = '';
      var args = Array.prototype.slice.call(arguments);
      if (args.length >= 2) {
        args[1] = normalizedUrl;
      }
      return originalOpen.apply(this, args);
    };

    XMLHttpRequest.prototype.setRequestHeader = function (name, value) {
      if (String(name || '').toLowerCase() === 'authorization') {
        this.__cliproxyPatchAuth = String(value || '');
      }
      return originalSetRequestHeader.apply(this, arguments);
    };

    XMLHttpRequest.prototype.send = function () {
      var xhr = this;
      xhr.addEventListener('loadend', function onLoadEnd() {
        if (xhr.status === 401) {
          clearSession();
          return;
        }
        if (xhr.status >= 200 && xhr.status < 400) {
          captureSession(xhr.__cliproxyPatchUrl, xhr.__cliproxyPatchAuth);
        }
      }, { once: true });
      return originalSend.apply(this, arguments);
    };
  }

  if (window.fetch) {
    var originalFetch = window.fetch;
    window.fetch = function (input, init) {
      var requestUrl = typeof input === 'string' ? input : (input && typeof input.url === 'string' ? input.url : '');
      var normalizedRequestUrl = normalizeManagementRequestUrl(requestUrl);
      var authorization = '';

      if (init && init.headers) {
        if (typeof Headers !== 'undefined' && init.headers instanceof Headers) {
          authorization = init.headers.get('Authorization') || '';
        } else if (Array.isArray(init.headers)) {
          for (var i = 0; i < init.headers.length; i++) {
            var entry = init.headers[i];
            if (Array.isArray(entry) && String(entry[0] || '').toLowerCase() === 'authorization') {
              authorization = String(entry[1] || '');
              break;
            }
          }
        } else if (typeof init.headers === 'object') {
          for (var headerName in init.headers) {
            if (Object.prototype.hasOwnProperty.call(init.headers, headerName) && String(headerName).toLowerCase() === 'authorization') {
              authorization = String(init.headers[headerName] || '');
              break;
            }
          }
        }
      }

      if (!authorization && input && typeof input === 'object' && typeof input.headers !== 'undefined') {
        try {
          if (typeof Headers !== 'undefined' && input.headers instanceof Headers) {
            authorization = input.headers.get('Authorization') || '';
          } else if (input.headers && typeof input.headers.get === 'function') {
            authorization = input.headers.get('Authorization') || '';
          }
        } catch (_err) {}
      }

      if (normalizedRequestUrl && normalizedRequestUrl !== requestUrl) {
        if (typeof input === 'string') {
          input = normalizedRequestUrl;
        } else if (typeof Request !== 'undefined' && input instanceof Request) {
          input = new Request(normalizedRequestUrl, input);
        } else {
          input = normalizedRequestUrl;
        }
        requestUrl = normalizedRequestUrl;
      }

      return originalFetch.call(this, input, init).then(function (response) {
        if (response && response.status === 401) {
          clearSession();
        } else {
          captureSession(requestUrl, authorization);
        }
        return response;
      });
    };
  }
})();
</script>`

// ManagementFileName exposes the control panel asset filename.
const ManagementFileName = managementAssetName

var (
	lastUpdateCheckMu   sync.Mutex
	lastUpdateCheckTime time.Time
	currentConfigPtr    atomic.Pointer[config.Config]
	schedulerOnce       sync.Once
	schedulerConfigPath atomic.Value
	sfGroup             singleflight.Group
)

// SetCurrentConfig stores the latest configuration snapshot for management asset decisions.
func SetCurrentConfig(cfg *config.Config) {
	if cfg == nil {
		currentConfigPtr.Store(nil)
		return
	}
	currentConfigPtr.Store(cfg)
}

// StartAutoUpdater launches a background goroutine that periodically ensures the management asset is up to date.
// It respects the disable-control-panel flag on every iteration and supports hot-reloaded configurations.
func StartAutoUpdater(ctx context.Context, configFilePath string) {
	configFilePath = strings.TrimSpace(configFilePath)
	if configFilePath == "" {
		log.Debug("management asset auto-updater skipped: empty config path")
		return
	}

	schedulerConfigPath.Store(configFilePath)

	schedulerOnce.Do(func() {
		go runAutoUpdater(ctx)
	})
}

func runAutoUpdater(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}

	ticker := time.NewTicker(updateCheckInterval)
	defer ticker.Stop()

	runOnce := func() {
		cfg := currentConfigPtr.Load()
		if cfg == nil {
			log.Debug("management asset auto-updater skipped: config not yet available")
			return
		}
		if cfg.RemoteManagement.DisableControlPanel {
			log.Debug("management asset auto-updater skipped: control panel disabled")
			return
		}
		if cfg.RemoteManagement.DisableAutoUpdatePanel {
			log.Debug("management asset auto-updater skipped: disable-auto-update-panel is enabled")
			return
		}

		configPath, _ := schedulerConfigPath.Load().(string)
		staticDir := StaticDir(configPath)
		EnsureLatestManagementHTML(ctx, staticDir, cfg.ProxyURL, cfg.RemoteManagement.PanelGitHubRepository)
	}

	runOnce()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runOnce()
		}
	}
}

func newHTTPClient(proxyURL string) *http.Client {
	client := &http.Client{Timeout: 15 * time.Second}

	sdkCfg := &sdkconfig.SDKConfig{ProxyURL: strings.TrimSpace(proxyURL)}
	util.SetProxy(sdkCfg, client)

	return client
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

type releaseResponse struct {
	Assets []releaseAsset `json:"assets"`
}

// StaticDir resolves the directory that stores the management control panel asset.
func StaticDir(configFilePath string) string {
	if override := strings.TrimSpace(os.Getenv("MANAGEMENT_STATIC_PATH")); override != "" {
		cleaned := filepath.Clean(override)
		if strings.EqualFold(filepath.Base(cleaned), managementAssetName) {
			return filepath.Dir(cleaned)
		}
		return cleaned
	}

	if writable := util.WritablePath(); writable != "" {
		return filepath.Join(writable, "static")
	}

	configFilePath = strings.TrimSpace(configFilePath)
	if configFilePath == "" {
		return ""
	}

	base := filepath.Dir(configFilePath)
	fileInfo, err := os.Stat(configFilePath)
	if err == nil {
		if fileInfo.IsDir() {
			base = configFilePath
		}
	}

	return filepath.Join(base, "static")
}

// FilePath resolves the absolute path to the management control panel asset.
func FilePath(configFilePath string) string {
	if override := strings.TrimSpace(os.Getenv("MANAGEMENT_STATIC_PATH")); override != "" {
		cleaned := filepath.Clean(override)
		if strings.EqualFold(filepath.Base(cleaned), managementAssetName) {
			return cleaned
		}
		return filepath.Join(cleaned, ManagementFileName)
	}

	dir := StaticDir(configFilePath)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, ManagementFileName)
}

// EnsureLatestManagementHTML checks the latest management.html asset and updates the local copy when needed.
// It coalesces concurrent sync attempts and returns whether the asset exists after the sync attempt.
func EnsureLatestManagementHTML(ctx context.Context, staticDir string, proxyURL string, panelRepository string) bool {
	if ctx == nil {
		ctx = context.Background()
	}

	staticDir = strings.TrimSpace(staticDir)
	if staticDir == "" {
		log.Debug("management asset sync skipped: empty static directory")
		return false
	}
	localPath := filepath.Join(staticDir, managementAssetName)

	_, _, _ = sfGroup.Do(localPath, func() (interface{}, error) {
		lastUpdateCheckMu.Lock()
		now := time.Now()
		timeSinceLastAttempt := now.Sub(lastUpdateCheckTime)
		if !lastUpdateCheckTime.IsZero() && timeSinceLastAttempt < managementSyncMinInterval {
			lastUpdateCheckMu.Unlock()
			log.Debugf(
				"management asset sync skipped by throttle: last attempt %v ago (interval %v)",
				timeSinceLastAttempt.Round(time.Second),
				managementSyncMinInterval,
			)
			return nil, nil
		}
		lastUpdateCheckTime = now
		lastUpdateCheckMu.Unlock()

		localFileMissing := false
		if _, errStat := os.Stat(localPath); errStat != nil {
			if errors.Is(errStat, os.ErrNotExist) {
				localFileMissing = true
			} else {
				log.WithError(errStat).Debug("failed to stat local management asset")
			}
		}

		if errMkdirAll := os.MkdirAll(staticDir, 0o755); errMkdirAll != nil {
			log.WithError(errMkdirAll).Warn("failed to prepare static directory for management asset")
			return nil, nil
		}

		releaseURL := resolveReleaseURL(panelRepository)
		client := newHTTPClient(proxyURL)

		localHash, err := fileSHA256(localPath)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				log.WithError(err).Debug("failed to read local management asset hash")
			}
			localHash = ""
		}

		asset, remoteHash, err := fetchLatestAsset(ctx, client, releaseURL)
		if err != nil {
			if localFileMissing {
				log.WithError(err).Warn("failed to fetch latest management release information, trying fallback page")
				if ensureFallbackManagementHTML(ctx, client, localPath) {
					return nil, nil
				}
				return nil, nil
			}
			log.WithError(err).Warn("failed to fetch latest management release information")
			return nil, nil
		}

		if remoteHash != "" && localHash != "" && strings.EqualFold(remoteHash, localHash) {
			log.Debug("management asset is already up to date")
			return nil, nil
		}

		data, downloadedHash, err := downloadAsset(ctx, client, asset.BrowserDownloadURL)
		if err != nil {
			if localFileMissing {
				log.WithError(err).Warn("failed to download management asset, trying fallback page")
				if ensureFallbackManagementHTML(ctx, client, localPath) {
					return nil, nil
				}
				return nil, nil
			}
			log.WithError(err).Warn("failed to download management asset")
			return nil, nil
		}

		if remoteHash != "" && !strings.EqualFold(remoteHash, downloadedHash) {
			log.Errorf("management asset digest mismatch: expected %s got %s — aborting update for safety", remoteHash, downloadedHash)
			return nil, nil
		}

		if err = atomicWriteFile(localPath, data); err != nil {
			log.WithError(err).Warn("failed to update management asset on disk")
			return nil, nil
		}

		log.Infof("management asset updated successfully (hash=%s)", downloadedHash)
		return nil, nil
	})

	_, err := os.Stat(localPath)
	return err == nil
}

func ensureFallbackManagementHTML(ctx context.Context, client *http.Client, localPath string) bool {
	data, downloadedHash, err := downloadAsset(ctx, client, defaultManagementFallbackURL)
	if err != nil {
		log.WithError(err).Warn("failed to download fallback management control panel page")
		return false
	}

	log.Warnf("management asset downloaded from fallback URL without digest verification (hash=%s) — "+
		"enable verified GitHub updates by keeping disable-auto-update-panel set to false", downloadedHash)

	if err = atomicWriteFile(localPath, data); err != nil {
		log.WithError(err).Warn("failed to persist fallback management control panel page")
		return false
	}

	log.Infof("management asset updated from fallback page successfully (hash=%s)", downloadedHash)
	return true
}

func resolveReleaseURL(repo string) string {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return defaultManagementReleaseURL
	}

	parsed, err := url.Parse(repo)
	if err != nil || parsed.Host == "" {
		return defaultManagementReleaseURL
	}

	host := strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")

	if host == "api.github.com" {
		if !strings.HasSuffix(strings.ToLower(parsed.Path), "/releases/latest") {
			parsed.Path = parsed.Path + "/releases/latest"
		}
		return parsed.String()
	}

	if host == "github.com" {
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
			repoName := strings.TrimSuffix(parts[1], ".git")
			return fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", parts[0], repoName)
		}
	}

	return defaultManagementReleaseURL
}

func fetchLatestAsset(ctx context.Context, client *http.Client, releaseURL string) (*releaseAsset, string, error) {
	if strings.TrimSpace(releaseURL) == "" {
		releaseURL = defaultManagementReleaseURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", httpUserAgent)
	gitURL := strings.ToLower(strings.TrimSpace(os.Getenv("GITSTORE_GIT_URL")))
	if tok := strings.TrimSpace(os.Getenv("GITSTORE_GIT_TOKEN")); tok != "" && strings.Contains(gitURL, "github.com") {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("execute release request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, "", fmt.Errorf("unexpected release status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var release releaseResponse
	if err = json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, "", fmt.Errorf("decode release response: %w", err)
	}

	for i := range release.Assets {
		asset := &release.Assets[i]
		if strings.EqualFold(asset.Name, managementAssetName) {
			remoteHash := parseDigest(asset.Digest)
			return asset, remoteHash, nil
		}
	}

	return nil, "", fmt.Errorf("management asset %s not found in latest release", managementAssetName)
}

func downloadAsset(ctx context.Context, client *http.Client, downloadURL string) ([]byte, string, error) {
	if strings.TrimSpace(downloadURL) == "" {
		return nil, "", fmt.Errorf("empty download url")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create download request: %w", err)
	}
	req.Header.Set("User-Agent", httpUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("execute download request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, "", fmt.Errorf("unexpected download status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAssetDownloadSize+1))
	if err != nil {
		return nil, "", fmt.Errorf("read download body: %w", err)
	}
	if int64(len(data)) > maxAssetDownloadSize {
		return nil, "", fmt.Errorf("download exceeds maximum allowed size of %d bytes", maxAssetDownloadSize)
	}

	sum := sha256.Sum256(data)
	return data, hex.EncodeToString(sum[:]), nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = file.Close()
	}()

	h := sha256.New()
	if _, err = io.Copy(h, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func atomicWriteFile(path string, data []byte) error {
	tmpFile, err := os.CreateTemp(filepath.Dir(path), "management-*.html")
	if err != nil {
		return err
	}

	tmpName := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
	}()

	if _, err = tmpFile.Write(data); err != nil {
		return err
	}

	if err = tmpFile.Chmod(0o644); err != nil {
		return err
	}

	if err = tmpFile.Close(); err != nil {
		return err
	}

	if err = os.Rename(tmpName, path); err != nil {
		return err
	}

	return nil
}

// PatchManagementHTML injects compatibility shims into the management UI.
func PatchManagementHTML(data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	content := string(data)
	injection := ""
	if !strings.Contains(content, managementSessionPatchMarker) {
		injection += managementSessionPatchMarker + managementSessionPatchScript + managementRouteFlashGuardScript
	}
	if !strings.Contains(content, managementKeyUsagePanelMarker) {
		injection += managementKeyUsagePanelPatch
	}
	if injection == "" {
		return data
	}

	lowerContent := strings.ToLower(content)

	if headIdx := strings.Index(lowerContent, "<head"); headIdx >= 0 {
		if tagEnd := strings.Index(lowerContent[headIdx:], ">"); tagEnd >= 0 {
			insertAt := headIdx + tagEnd + 1
			return []byte(content[:insertAt] + injection + content[insertAt:])
		}
	}
	if idx := strings.Index(lowerContent, "<script"); idx >= 0 {
		return []byte(content[:idx] + injection + content[idx:])
	}
	if idx := strings.Index(lowerContent, "</head>"); idx >= 0 {
		return []byte(content[:idx] + injection + content[idx:])
	}
	if idx := strings.Index(lowerContent, "</body>"); idx >= 0 {
		return []byte(content[:idx] + injection + content[idx:])
	}

	return []byte(content + injection)
}

func parseDigest(digest string) string {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return ""
	}

	if idx := strings.Index(digest, ":"); idx >= 0 {
		digest = digest[idx+1:]
	}

	return strings.ToLower(strings.TrimSpace(digest))
}
