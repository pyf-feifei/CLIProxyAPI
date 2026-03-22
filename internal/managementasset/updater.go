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

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/util"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"
)

const (
	defaultManagementReleaseURL  = "https://api.github.com/repos/router-for-me/Cli-Proxy-API-Management-Center/releases/latest"
	defaultManagementFallbackURL = "https://cpamc.router-for.me/"
	managementAssetName          = "management.html"
	httpUserAgent                = "CLIProxyAPI-management-updater"
	managementSyncMinInterval    = 30 * time.Second
	updateCheckInterval          = 3 * time.Hour
	managementSessionPatchMarker = "<!-- cliproxyapi-session-refresh-patch -->"
)

const managementSessionPatchScript = `<script>(function(){if(window.__cliproxySessionPatchV1){return;}window.__cliproxySessionPatchV1=true;var SESSION_KEY='cli-proxy-session-auth';var ENC_PREFIX='enc::v1::';var SECRET_SALT='cli-proxy-api-webui::secure-storage';var recentLoginCaptureAt=0;var recentUserInteractionAt=0;function encodeText(text){return new TextEncoder().encode(text);}function decodeText(bytes){return new TextDecoder().decode(bytes);}function getKeyBytes(){try{return encodeText(SECRET_SALT+'|'+window.location.host+'|'+navigator.userAgent);}catch(_err){return encodeText(SECRET_SALT);}}function xorBytes(data,keyBytes){var result=new Uint8Array(data.length);for(var i=0;i<data.length;i++){result[i]=data[i]^keyBytes[i%keyBytes.length];}return result;}function toBase64(bytes){var binary='';for(var i=0;i<bytes.length;i++){binary+=String.fromCharCode(bytes[i]);}return btoa(binary);}function fromBase64(base64){var binary=atob(base64);var bytes=new Uint8Array(binary.length);for(var i=0;i<binary.length;i++){bytes[i]=binary.charCodeAt(i);}return bytes;}function encryptData(value){if(!value){return value;}try{return ENC_PREFIX+toBase64(xorBytes(encodeText(value),getKeyBytes()));}catch(_err){return value;}}function decryptData(payload){if(!payload||payload.indexOf(ENC_PREFIX)!==0){return payload;}try{return decodeText(xorBytes(fromBase64(payload.slice(ENC_PREFIX.length)),getKeyBytes()));}catch(_err){return payload;}}function encodeStoredValue(value){return encryptData(JSON.stringify(value));}function decodeStoredValue(raw){var payload=raw;if(payload&&payload.indexOf(ENC_PREFIX)===0){payload=decryptData(payload);}return JSON.parse(payload);}function readSession(){try{var raw=window.sessionStorage.getItem(SESSION_KEY);if(!raw){return null;}var parsed=JSON.parse(raw);if(!parsed||typeof parsed.apiBase!=='string'||typeof parsed.managementKey!=='string'){return null;}return{apiBase:parsed.apiBase,managementKey:parsed.managementKey,sessionOnly:parsed.sessionOnly===true,updatedAt:Number(parsed.updatedAt||0)};}catch(_err){return null;}}function writeSession(apiBase,managementKey,sessionOnly){apiBase=String(apiBase||'').trim();managementKey=String(managementKey||'').trim();if(!apiBase||!managementKey){return;}try{window.sessionStorage.setItem(SESSION_KEY,JSON.stringify({apiBase:apiBase,managementKey:managementKey,sessionOnly:sessionOnly===true,updatedAt:Date.now()}));}catch(_err){}}function updateSessionMode(sessionOnly){var current=readSession();if(!current){return;}writeSession(current.apiBase,current.managementKey,sessionOnly);}function clearSession(){try{window.sessionStorage.removeItem(SESSION_KEY);}catch(_err){}}function normalizeApiBase(requestUrl){try{var url=new URL(requestUrl,window.location.href);var marker='/v0/management';var lowerPath=url.pathname.toLowerCase();var idx=lowerPath.indexOf(marker);var basePath=idx>=0?url.pathname.slice(0,idx):url.pathname;basePath=basePath.replace(/\/+$/,'');return url.origin+basePath;}catch(_err){return'';}}function isManagementURL(requestUrl){return String(requestUrl||'').toLowerCase().indexOf('/v0/management')>=0;}function looksLikeManualLogin(){return(window.location.hash||'').indexOf('#/login')===0&&(Date.now()-recentUserInteractionAt)<5000;}function sanitizePersistedAuth(raw){try{var parsed=decodeStoredValue(raw);if(!parsed||typeof parsed!=='object'||!parsed.state||typeof parsed.state!=='object'){return raw;}parsed.state.rememberPassword=false;delete parsed.state.managementKey;return encodeStoredValue(parsed);}catch(_err){return raw;}}function payloadPersistsManagementKey(raw){try{var parsed=decodeStoredValue(raw);if(!parsed||typeof parsed!=='object'||!parsed.state||typeof parsed.state!=='object'){return false;}return parsed.state.rememberPassword===true||(typeof parsed.state.managementKey==='string'&&parsed.state.managementKey.length>0);}catch(_err){return false;}}function captureSession(requestUrl,authorization){if(!isManagementURL(requestUrl)){return;}var match=String(authorization||'').match(/^\s*Bearer\s+(.+?)\s*$/i);if(!match||!match[1]){return;}var apiBase=normalizeApiBase(requestUrl);if(!apiBase){return;}var current=readSession();writeSession(apiBase,match[1],current?current.sessionOnly:false);if((window.location.hash||'').indexOf('#/login')===0){recentLoginCaptureAt=Date.now();}}function markInteraction(){recentUserInteractionAt=Date.now();}window.addEventListener('pointerdown',markInteraction,true);window.addEventListener('keydown',markInteraction,true);window.addEventListener('submit',markInteraction,true);var originalGetItem=Storage.prototype.getItem;var originalSetItem=Storage.prototype.setItem;var originalRemoveItem=Storage.prototype.removeItem;Storage.prototype.getItem=function(key){if(this===window.localStorage){var session=readSession();if(session&&session.sessionOnly){if(key==='isLoggedIn'){return'true';}if(key==='apiBase'||key==='apiUrl'){return encodeStoredValue(session.apiBase);}if(key==='managementKey'){return encodeStoredValue(session.managementKey);}}}return originalGetItem.call(this,key);};Storage.prototype.setItem=function(key,value){if(this===window.localStorage){var session=readSession();if(key==='cli-proxy-auth'&&session&&session.sessionOnly){if(looksLikeManualLogin()&&payloadPersistsManagementKey(String(value))){updateSessionMode(false);return originalSetItem.call(this,key,value);}return originalSetItem.call(this,key,sanitizePersistedAuth(String(value)));}if(key==='isLoggedIn'&&value==='true'){if(session&&session.sessionOnly){if(looksLikeManualLogin()){updateSessionMode(false);return originalSetItem.call(this,key,value);}return;}if(session){updateSessionMode(false);}}}return originalSetItem.call(this,key,value);};Storage.prototype.removeItem=function(key){if(this===window.localStorage&&key==='isLoggedIn'){var looksLikeFreshSessionOnlyLogin=(window.location.hash||'').indexOf('#/login')===0&&(Date.now()-recentLoginCaptureAt)<3000;if(looksLikeFreshSessionOnlyLogin){updateSessionMode(true);}else{clearSession();}}return originalRemoveItem.call(this,key);};window.addEventListener('unauthorized',clearSession);if(window.XMLHttpRequest){var originalOpen=XMLHttpRequest.prototype.open;var originalSetRequestHeader=XMLHttpRequest.prototype.setRequestHeader;var originalSend=XMLHttpRequest.prototype.send;XMLHttpRequest.prototype.open=function(method,url){this.__cliproxyPatchUrl=url;this.__cliproxyPatchAuth='';return originalOpen.apply(this,arguments);};XMLHttpRequest.prototype.setRequestHeader=function(name,value){if(String(name||'').toLowerCase()==='authorization'){this.__cliproxyPatchAuth=String(value||'');}return originalSetRequestHeader.apply(this,arguments);};XMLHttpRequest.prototype.send=function(){var xhr=this;xhr.addEventListener('loadend',function onLoadEnd(){if(xhr.status===401){clearSession();return;}if(xhr.status>=200&&xhr.status<400){captureSession(xhr.__cliproxyPatchUrl,xhr.__cliproxyPatchAuth);}},{once:true});return originalSend.apply(this,arguments);};}if(window.fetch){var originalFetch=window.fetch;window.fetch=function(input,init){var requestUrl=typeof input==='string'?input:(input&&typeof input.url==='string'?input.url:'');var authorization='';if(init&&init.headers){if(typeof Headers!=='undefined'&&init.headers instanceof Headers){authorization=init.headers.get('Authorization')||'';}else if(Array.isArray(init.headers)){for(var i=0;i<init.headers.length;i++){var entry=init.headers[i];if(Array.isArray(entry)&&String(entry[0]||'').toLowerCase()==='authorization'){authorization=String(entry[1]||'');break;}}}else if(typeof init.headers==='object'){for(var headerName in init.headers){if(Object.prototype.hasOwnProperty.call(init.headers,headerName)&&String(headerName).toLowerCase()==='authorization'){authorization=String(init.headers[headerName]||'');break;}}}}if(!authorization&&input&&typeof input==='object'&&typeof input.headers!=='undefined'){try{if(typeof Headers!=='undefined'&&input.headers instanceof Headers){authorization=input.headers.get('Authorization')||'';}else if(input.headers&&typeof input.headers.get==='function'){authorization=input.headers.get('Authorization')||'';}}catch(_err){}}return originalFetch.apply(this,arguments).then(function(response){if(response&&response.status===401){clearSession();}else{captureSession(requestUrl,authorization);}return response;});};}})();</script>`

const managementRouteFlashGuardScript = `<script>(function(){if(window.__cliproxyRouteFlashGuardV1){return;}window.__cliproxyRouteFlashGuardV1=true;var SESSION_KEY='cli-proxy-session-auth';var currentHash=window.location.hash||'';if(!currentHash||currentHash.indexOf('#/login')===0){return;}var hasSession=false;try{hasSession=!!window.sessionStorage.getItem(SESSION_KEY);}catch(_err){hasSession=false;}if(!hasSession){return;}var STYLE_ID='cliproxy-refresh-guard-style';function ensureStyle(){if(document.getElementById(STYLE_ID)){return;}var style=document.createElement('style');style.id=STYLE_ID;style.textContent='html[data-cliproxy-refresh-guard=\"on\"] #root{visibility:hidden !important;}';document.head.appendChild(style);}function clearGuard(){document.documentElement.removeAttribute('data-cliproxy-refresh-guard');window.removeEventListener('hashchange',onHashChange,true);if(interval){window.clearInterval(interval);interval=null;}}function isLoginHash(){return(window.location.hash||'').indexOf('#/login')===0;}function onHashChange(){if(!isLoginHash()){clearGuard();}}ensureStyle();document.documentElement.setAttribute('data-cliproxy-refresh-guard','on');window.addEventListener('hashchange',onHashChange,true);var interval=window.setInterval(function(){if(!isLoginHash()){clearGuard();}},100);window.setTimeout(clearGuard,4000);window.addEventListener('pageshow',function(){if(!isLoginHash()){clearGuard();}},{once:true});})();</script>`

const managementSessionPatchScriptV2 = `<script>
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
      return {
        apiBase: parsed.apiBase,
        managementKey: parsed.managementKey,
        sessionOnly: parsed.sessionOnly === true,
        updatedAt: Number(parsed.updatedAt || 0)
      };
    } catch (_err) {
      return null;
    }
  }

  function writeSession(apiBase, managementKey, sessionOnly) {
    apiBase = String(apiBase || '').trim();
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

  function normalizeApiBase(requestUrl) {
    try {
      var parsedUrl = new URL(requestUrl, window.location.href);
      var marker = '/v0/management';
      var lowerPath = parsedUrl.pathname.toLowerCase();
      var idx = lowerPath.indexOf(marker);
      var basePath = idx >= 0 ? parsedUrl.pathname.slice(0, idx) : parsedUrl.pathname;
      basePath = basePath.replace(/\/+$/, '');
      return parsedUrl.origin + basePath;
    } catch (_err) {
      return '';
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

  function payloadPersistsManagementKey(raw) {
    try {
      var parsed = decodeStoredValue(raw);
      if (!parsed || typeof parsed !== 'object' || !parsed.state || typeof parsed.state !== 'object') {
        return false;
      }
      return parsed.state.rememberPassword === true || (typeof parsed.state.managementKey === 'string' && parsed.state.managementKey.length > 0);
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
    parsed.state.managementKey = session.managementKey;
    parsed.state.rememberPassword = false;
    parsed.state.isAuthenticated = true;

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
    writeSession(apiBase, match[1], current ? current.sessionOnly : false);

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
        if (looksLikeManualLogin() && payloadPersistsManagementKey(String(value))) {
          updateSessionMode(false);
          return originalSetItem.call(this, key, value);
        }
        return originalSetItem.call(this, key, sanitizePersistedAuth(String(value)));
      }

      if (key === 'isLoggedIn' && value === 'true') {
        if (session && session.sessionOnly) {
          if (looksLikeManualLogin()) {
            updateSessionMode(false);
            return originalSetItem.call(this, key, value);
          }
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
      this.__cliproxyPatchUrl = url;
      this.__cliproxyPatchAuth = '';
      return originalOpen.apply(this, arguments);
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

      return originalFetch.apply(this, arguments).then(function (response) {
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
			log.Warnf("remote digest mismatch for management asset: expected %s got %s", remoteHash, downloadedHash)
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

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read download body: %w", err)
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

// PatchManagementHTML injects a session-only login restore shim into the management UI.
// The upstream UI currently ties page-refresh restoration to "remember password";
// this patch keeps the session alive across refreshes within the same tab only.
func PatchManagementHTML(data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	content := string(data)
	if strings.Contains(content, managementSessionPatchMarker) {
		return data
	}

	injection := managementSessionPatchMarker + managementSessionPatchScriptV2 + managementRouteFlashGuardScript
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
