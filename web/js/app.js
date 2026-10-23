// CoreIP Server Inspector - Main Application Controller

const CoreIPApp = {
  state: {
    currentView: 'dashboard',
    pollIntervalSeconds: 5,
    pollTimer: null,
    inventory: null,
    history: [],
    session: null,
    csrfToken: '',
    serviceFilter: '',
    servicePage: 1,
    servicePageSize: 25,
    theme: localStorage.getItem('coreip_theme') || 'dark'
  },

  init() {
    this.applyTheme(this.state.theme);
    this.setupNavigation();
    this.setupEventListeners();
    this.checkSession().then(() => {
      if (!this.state.session || !this.state.session.auth_enabled || this.state.session.authenticated) {
        this.refreshAll();
        this.startPolling();
      } else {
        const updateEl = document.getElementById('last-updated-text');
        if (updateEl) updateEl.textContent = 'Authentication required';
        this.showLoginModal();
      }
    });
  },

  applyTheme(theme) {
    this.state.theme = theme;
    document.documentElement.setAttribute('data-theme', theme);
    localStorage.setItem('coreip_theme', theme);
    const themeBtn = document.getElementById('btn-toggle-theme');
    if (themeBtn) {
      themeBtn.textContent = theme === 'dark' ? '☀️ Light' : '🌙 Dark';
    }
  },

  toggleTheme() {
    const nextTheme = this.state.theme === 'dark' ? 'light' : 'dark';
    this.applyTheme(nextTheme);
  },

  setupNavigation() {
    const links = document.querySelectorAll('.nav-link');
    links.forEach(link => {
      link.addEventListener('click', (e) => {
        e.preventDefault();
        const targetView = link.getAttribute('data-view');
        this.switchView(targetView);
      });
    });
  },

  switchView(viewName) {
    this.state.currentView = viewName;
    document.querySelectorAll('.nav-link').forEach(l => {
      l.classList.toggle('active', l.getAttribute('data-view') === viewName);
    });

    document.querySelectorAll('.view-section').forEach(sec => {
      sec.classList.toggle('active', sec.id === `view-${viewName}`);
    });

    // Render current view with cached inventory
    if (this.state.inventory) {
      this.renderCurrentView();
    }
  },

  renderCurrentView() {
    const inv = this.state.inventory;
    if (!inv) return;

    switch (this.state.currentView) {
      case 'dashboard':
        CoreIPViews.renderDashboard(inv, this.state.history);
        break;
      case 'system':
        CoreIPViews.renderSystem(inv.system);
        break;
      case 'cpu-memory':
        CoreIPViews.renderCPUMemory(inv.cpu, inv.memory);
        break;
      case 'storage':
        CoreIPViews.renderStorage(inv.storage, inv.smart);
        break;
      case 'gpu':
        CoreIPViews.renderGPU(inv.gpu);
        break;
      case 'network':
        CoreIPViews.renderNetwork(inv.network);
        break;
      case 'services':
        this.fetchServices();
        break;
      case 'health':
        CoreIPViews.renderHealth(inv.health);
        break;
      case 'reports':
        this.loadReportPreview();
        break;
      case 'settings':
        this.fetchConfig();
        break;
    }
  },

  async apiRequest(endpoint, options = {}) {
    options.headers = options.headers || {};
    if (this.state.csrfToken) {
      options.headers['X-CSRF-Token'] = this.state.csrfToken;
    }

    try {
      const resp = await fetch(endpoint, options);
      if (resp.status === 401) {
        this.showLoginModal();
        throw new Error('Authentication required');
      }
      if (!resp.ok) {
        const errJson = await resp.json().catch(() => ({ error: resp.statusText }));
        throw new Error(errJson.error || `HTTP ${resp.status}`);
      }
      return resp;
    } catch (err) {
      throw err;
    }
  },

  async checkSession() {
    try {
      const res = await fetch('/api/v1/auth/session');
      if (res.ok) {
        const data = await res.json();
        this.state.session = data;
        if (data.csrf_token) {
          this.state.csrfToken = data.csrf_token;
        }
        this.updateAuthUI(data);
      }
    } catch (e) {
      console.warn('Session check error', e);
    }
  },

  updateAuthUI(sess) {
    const authBtn = document.getElementById('btn-auth');
    if (!authBtn) return;

    if (sess && sess.authenticated) {
      authBtn.textContent = `Logout (${sess.username})`;
      authBtn.onclick = () => this.logout();
    } else if (sess && !sess.auth_enabled && sess.is_loopback) {
      authBtn.textContent = 'Local (Unauthenticated)';
      authBtn.onclick = null;
    } else {
      authBtn.textContent = 'Login';
      authBtn.onclick = () => this.showLoginModal();
    }
  },

  async refreshAll() {
    const updateEl = document.getElementById('last-updated-text');
    if (updateEl) updateEl.textContent = 'Refreshing...';

    try {
      // 1. Fetch full inventory
      const resInv = await this.apiRequest('/api/v1/hardware');
      this.state.inventory = await resInv.json();

      // 2. Fetch history
      const resHist = await this.apiRequest('/api/v1/metrics/history');
      const histData = await resHist.json();
      this.state.history = histData.history || [];

      // Update global health dot
      const health = this.state.inventory.health || {};
      const dotEl = document.getElementById('global-health-dot');
      if (dotEl) {
        dotEl.className = 'dot ' + (health.overall_status === 'healthy' ? 'dot-success' : (health.overall_status === 'warning' ? 'dot-warning' : 'dot-danger'));
      }
      const badgeEl = document.getElementById('global-health-badge');
      if (badgeEl) {
        badgeEl.textContent = (health.overall_status || 'HEALTHY').toUpperCase();
        badgeEl.className = 'badge ' + (health.overall_status === 'healthy' ? 'badge-success' : (health.overall_status === 'warning' ? 'badge-warning' : 'badge-danger'));
      }

      this.renderCurrentView();

      if (updateEl) {
        updateEl.textContent = `Updated: ${new Date().toLocaleTimeString()}`;
      }
    } catch (err) {
      if (updateEl) updateEl.textContent = 'Error refreshing';
      console.error('Refresh error:', err);
    }
  },

  startPolling() {
    if (this.state.pollTimer) clearInterval(this.state.pollTimer);
    if (this.state.pollIntervalSeconds <= 0) return;

    this.state.pollTimer = setInterval(async () => {
      // Don't poll if unauthenticated and auth is enabled
      if (this.state.session && this.state.session.auth_enabled && !this.state.session.authenticated) {
        return;
      }

      // Only fetch live metrics + history on poll ticks for efficiency
      try {
        const resHist = await this.apiRequest('/api/v1/metrics/history');
        const histData = await resHist.json();
        this.state.history = histData.history || [];

        // Also fetch live current point
        const resCur = await this.apiRequest('/api/v1/metrics/current');
        const curPoint = await resCur.json();

        if (this.state.inventory) {
          this.state.inventory.cpu.current_utilization_pct = curPoint.cpu_utilization_pct;
          this.state.inventory.memory.utilization_pct = curPoint.memory_usage_pct;
          this.state.inventory.cpu.load_averages[0] = curPoint.load_1m;
        }

        if (this.state.currentView === 'dashboard') {
          CoreIPViews.renderDashboard(this.state.inventory, this.state.history);
        }
      } catch (err) {
        // Suppress repeated background error spam
      }
    }, this.state.pollIntervalSeconds * 1000);
  },

  async fetchServices() {
    try {
      const url = `/api/v1/services?filter=${encodeURIComponent(this.state.serviceFilter)}&page=${this.state.servicePage}&page_size=${this.state.servicePageSize}`;
      const res = await this.apiRequest(url);
      const srvData = await res.json();
      CoreIPViews.renderServices(srvData);
    } catch (err) {
      console.error('Failed to fetch services:', err);
    }
  },

  async fetchConfig() {
    try {
      const res = await this.apiRequest('/api/v1/config');
      const data = await res.json();
      CoreIPViews.renderSettings(data.config, this.state.session);
    } catch (err) {
      console.error('Failed to fetch config:', err);
    }
  },

  async loadReportPreview() {
    const previewEl = document.getElementById('report-preview-box');
    if (!previewEl) return;
    previewEl.textContent = 'Generating system report...';
    try {
      const res = await this.apiRequest('/api/v1/report?format=txt');
      const text = await res.text();
      previewEl.textContent = text;
    } catch (err) {
      previewEl.textContent = 'Error loading report preview: ' + err.message;
    }
  },

  showLoginModal() {
    const modal = document.getElementById('login-modal');
    if (modal) modal.classList.add('active');
  },

  hideLoginModal() {
    const modal = document.getElementById('login-modal');
    if (modal) modal.classList.remove('active');
  },

  async login(username, password) {
    try {
      const resp = await fetch('/api/v1/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password })
      });

      if (!resp.ok) {
        const errJson = await resp.json().catch(() => ({ error: 'Login failed' }));
        this.showToast(errJson.error || 'Login failed', 'danger');
        return;
      }

      const data = await resp.json();
      this.state.csrfToken = data.csrf_token;
      this.hideLoginModal();
      this.showToast(`Logged in successfully as ${data.username}`, 'success');
      await this.checkSession();
      this.refreshAll();
      this.startPolling();
    } catch (err) {
      this.showToast(err.message, 'danger');
    }
  },

  async logout() {
    try {
      await this.apiRequest('/api/v1/auth/logout', { method: 'POST' });
      this.state.session = null;
      this.state.csrfToken = '';
      this.showToast('Logged out', 'info');
      await this.checkSession();
      this.refreshAll();
    } catch (err) {
      this.showToast('Logout failed: ' + err.message, 'danger');
    }
  },

  showToast(message, type = 'info') {
    const container = document.getElementById('toast-container');
    if (!container) return;

    const toast = document.createElement('div');
    toast.className = `toast alert-${type}`;
    toast.innerHTML = `<span>${CoreIPViews.escapeHTML(message)}</span>`;
    container.appendChild(toast);

    setTimeout(() => {
      toast.remove();
    }, 4000);
  },

  setupEventListeners() {
    // Manual Refresh
    document.getElementById('btn-refresh').addEventListener('click', () => {
      this.refreshAll();
      this.showToast('Inventory refreshed', 'info');
    });

    // Theme Toggle
    document.getElementById('btn-toggle-theme').addEventListener('click', () => {
      this.toggleTheme();
    });

    // Poll Interval Selector
    const pollSelect = document.getElementById('select-poll-interval');
    if (pollSelect) {
      pollSelect.value = this.state.pollIntervalSeconds;
      pollSelect.addEventListener('change', (e) => {
        this.state.pollIntervalSeconds = parseInt(e.target.value, 10);
        this.startPolling();
        this.showToast(`Polling cadence set to ${this.state.pollIntervalSeconds}s`, 'info');
      });
    }

    // Login Form Submit
    const loginForm = document.getElementById('login-form');
    if (loginForm) {
      loginForm.addEventListener('submit', (e) => {
        e.preventDefault();
        const u = document.getElementById('login-username').value;
        const p = document.getElementById('login-password').value;
        this.login(u, p);
      });
    }

    // Modal Close
    const closeBtn = document.getElementById('btn-close-modal');
    if (closeBtn) closeBtn.addEventListener('click', () => this.hideLoginModal());

    // Services Filter & Pagination
    const srvSearch = document.getElementById('srv-search-input');
    if (srvSearch) {
      let debounce;
      srvSearch.addEventListener('input', (e) => {
        clearTimeout(debounce);
        debounce = setTimeout(() => {
          this.state.serviceFilter = e.target.value;
          this.state.servicePage = 1;
          this.fetchServices();
        }, 300);
      });
    }

    const srvFilter = document.getElementById('srv-filter-status');
    if (srvFilter) {
      srvFilter.addEventListener('change', (e) => {
        this.state.serviceFilter = e.target.value;
        this.state.servicePage = 1;
        this.fetchServices();
      });
    }

    const prevBtn = document.getElementById('srv-prev-page');
    if (prevBtn) {
      prevBtn.addEventListener('click', () => {
        if (this.state.servicePage > 1) {
          this.state.servicePage--;
          this.fetchServices();
        }
      });
    }

    const nextBtn = document.getElementById('srv-next-page');
    if (nextBtn) {
      nextBtn.addEventListener('click', () => {
        this.state.servicePage++;
        this.fetchServices();
      });
    }

    // Download Report Buttons
    const btnDownloadTXT = document.getElementById('btn-download-txt');
    if (btnDownloadTXT) {
      btnDownloadTXT.addEventListener('click', () => {
        window.location.href = '/api/v1/report?format=txt&download=true';
      });
    }

    const btnDownloadJSON = document.getElementById('btn-download-json');
    if (btnDownloadJSON) {
      btnDownloadJSON.addEventListener('click', () => {
        window.location.href = '/api/v1/report?format=json&download=true';
      });
    }

    const btnCopyReport = document.getElementById('btn-copy-report');
    if (btnCopyReport) {
      btnCopyReport.addEventListener('click', async () => {
        const text = document.getElementById('report-preview-box').textContent;
        await navigator.clipboard.writeText(text);
        this.showToast('Report copied to clipboard', 'success');
      });
    }
  }
};

window.addEventListener('DOMContentLoaded', () => {
  CoreIPApp.init();
});

