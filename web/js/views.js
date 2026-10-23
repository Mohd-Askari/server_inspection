// CoreIP Server Inspector - View Renderers

class CoreIPViews {
  static formatBytes(b) {
    if (!b || b === 0) return '0 B';
    const unit = 1024;
    if (b < unit) return b + ' B';
    const exp = Math.floor(Math.log(b) / Math.log(unit));
    const pre = 'KMGTPE'[exp - 1];
    return (b / Math.pow(unit, exp)).toFixed(2) + ' ' + pre + 'iB';
  }

  static formatRate(bytesSec) {
    if (!bytesSec || bytesSec < 1) return '0 B/s';
    return this.formatBytes(bytesSec) + '/s';
  }

  static escapeHTML(str) {
    if (!str) return '';
    return String(str)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#039;');
  }

  // 1. Dashboard View
  static renderDashboard(inv, history) {
    if (!inv) return;
    const sys = inv.system || {};
    const cpu = inv.cpu || {};
    const mem = inv.memory || {};
    const stg = inv.storage || {};
    const srv = inv.services || {};
    const smt = inv.smart || {};
    const hlt = inv.health || {};

    // Summary Cards
    document.getElementById('dash-host').textContent = sys.hostname || '—';
    document.getElementById('dash-os').textContent = `${sys.os_name || ''} ${sys.os_version || ''}`;
    document.getElementById('dash-uptime').textContent = sys.uptime_formatted || '—';

    document.getElementById('dash-cpu-cores').textContent = `${cpu.physical_cores || '—'} / ${cpu.logical_processors || '—'}`;
    document.getElementById('dash-cpu-model').textContent = cpu.model_name || 'Generic CPU';

    document.getElementById('dash-ram-total').textContent = this.formatBytes(mem.total_bytes);
    document.getElementById('dash-ram-avail').textContent = `Avail: ${this.formatBytes(mem.available_bytes)}`;

    document.getElementById('dash-disk-cap').textContent = this.formatBytes(stg.total_physical_capacity_bytes);
    document.getElementById('dash-disk-count').textContent = `${(stg.disks || []).length} Drive(s)`;

    document.getElementById('dash-cpu-util').textContent = `${(cpu.current_utilization_pct || 0).toFixed(1)}%`;
    const cpuBar = document.getElementById('dash-cpu-bar');
    if (cpuBar) cpuBar.style.width = `${Math.min(cpu.current_utilization_pct || 0, 100)}%`;

    document.getElementById('dash-ram-util').textContent = `${(mem.utilization_pct || 0).toFixed(1)}%`;
    const ramBar = document.getElementById('dash-ram-bar');
    if (ramBar) ramBar.style.width = `${Math.min(mem.utilization_pct || 0, 100)}%`;

    const failedCount = srv.failed_services || 0;
    const failedEl = document.getElementById('dash-srv-failed');
    if (failedEl) {
      failedEl.textContent = failedCount;
      failedEl.className = failedCount > 0 ? 'stat-value color-danger' : 'stat-value color-success';
    }

    // SMART status summary
    let smartHealthy = true;
    let smartUnknown = false;
    (smt.devices || []).forEach(d => {
      if (d.overall_status === 'critical' || d.overall_status === 'warning') smartHealthy = false;
      if (d.overall_status === 'unavailable') smartUnknown = true;
    });
    const smartEl = document.getElementById('dash-smart-status');
    if (smartEl) {
      if (!smartHealthy) {
        smartEl.innerHTML = '<span class="badge badge-danger">Attention Needed</span>';
      } else if (smartUnknown) {
        smartEl.innerHTML = '<span class="badge badge-neutral">Diagnostics Unavailable</span>';
      } else {
        smartEl.innerHTML = '<span class="badge badge-success">All Healthy</span>';
      }
    }

    // Warnings Banner
    const alertsContainer = document.getElementById('dash-alerts-container');
    if (alertsContainer) {
      if (hlt.issues && hlt.issues.length > 0) {
        let alertHTML = '';
        hlt.issues.slice(0, 3).forEach(iss => {
          const cls = iss.severity === 'critical' ? 'alert-danger' : 'alert-warning';
          alertHTML += `<div class="alert-box ${cls}">
            <strong>[${this.escapeHTML(iss.component)}]</strong> ${this.escapeHTML(iss.message)}
          </div>`;
        });
        alertsContainer.innerHTML = alertHTML;
        alertsContainer.style.display = 'block';
      } else {
        alertsContainer.style.display = 'none';
      }
    }

    // Render Live Charts from History
    if (history && history.length > 0) {
      const cpuSeries = history.map(h => h.cpu_utilization_pct || 0);
      const ramSeries = history.map(h => h.memory_usage_pct || 0);
      const rxSeries = history.map(h => (h.network_rx_bytes_sec || 0) / 1024); // KiB/s
      const txSeries = history.map(h => (h.network_tx_bytes_sec || 0) / 1024);

      CoreIPCharts.drawLineChart('chart-cpu-history', [{
        data: cpuSeries,
        color: '#38bdf8',
        fill: true,
        fillColor: 'rgba(56, 189, 248, 0.2)'
      }], { max: 100, unit: '%' });

      CoreIPCharts.drawLineChart('chart-ram-history', [{
        data: ramSeries,
        color: '#10b981',
        fill: true,
        fillColor: 'rgba(16, 185, 129, 0.2)'
      }], { max: 100, unit: '%' });

      CoreIPCharts.drawLineChart('chart-net-history', [
        { data: rxSeries, color: '#6366f1', label: 'RX' },
        { data: txSeries, color: '#f59e0b', label: 'TX' }
      ], { unit: ' KiB/s' });
    }
  }

  // 2. System Information View
  static renderSystem(sys) {
    if (!sys) return;
    const kv = {
      'Hostname': sys.hostname,
      'FQDN': sys.fqdn || 'Not Configured',
      'Operating System': `${sys.os_name} (${sys.os_version})`,
      'OS Family': sys.os_family,
      'Kernel Version': sys.kernel_version,
      'Architecture': sys.architecture,
      'System Uptime': sys.uptime_formatted,
      'Boot Timestamp': sys.boot_time ? new Date(sys.boot_time).toLocaleString() : 'N/A',
      'Hardware Vendor': sys.manufacturer || 'Not Available',
      'Hardware Model': sys.model || 'Not Available',
      'Serial Number': sys.serial_number || 'Permission Restricted',
      'BIOS Version': sys.bios_version ? `${sys.bios_version} (${sys.bios_date || ''})` : 'N/A'
    };

    let html = '<div class="kv-grid">';
    for (const [k, v] of Object.entries(kv)) {
      html += `<div class="kv-item"><span class="kv-label">${k}</span><span class="kv-value">${this.escapeHTML(v)}</span></div>`;
    }
    html += '</div>';
    document.getElementById('sys-details-card').innerHTML = html;

    // Virtualization Details
    const virt = sys.virtualization || {};
    let virtBadge = virt.is_virtual ? '<span class="badge badge-info">Virtual Machine</span>' : '<span class="badge badge-success">Bare Metal</span>';
    let virtHTML = `
      <div style="display:flex; align-items:center; gap:0.75rem; margin-bottom:1rem;">
        ${virtBadge}
        <strong>${this.escapeHTML(virt.description || 'Physical Server')}</strong>
      </div>
      <div class="kv-grid">
        <div class="kv-item"><span class="kv-label">Role</span><span class="kv-value">${this.escapeHTML(virt.role || 'bare-metal')}</span></div>
        <div class="kv-item"><span class="kv-label">Hypervisor Platform</span><span class="kv-value">${this.escapeHTML(virt.hypervisor || 'none')}</span></div>
      </div>
    `;
    document.getElementById('sys-virt-card').innerHTML = virtHTML;
  }

  // 3. CPU and Memory View
  static renderCPUMemory(cpu, mem) {
    if (!cpu || !mem) return;

    // CPU Section
    const cpuKV = {
      'Processor Model': cpu.model_name,
      'Vendor Identifier': cpu.vendor_id,
      'Physical Sockets': cpu.physical_sockets,
      'Physical Cores': cpu.physical_cores,
      'Logical Processors': cpu.logical_processors,
      'Virtual vCPUs': cpu.vm_vcpus > 0 ? cpu.vm_vcpus : 'None (Physical)',
      'Current Frequency': cpu.current_freq_mhz > 0 ? `${cpu.current_freq_mhz.toFixed(1)} MHz` : 'Dynamic',
      'Load Averages': `1m: ${cpu.load_averages[0].toFixed(2)} | 5m: ${cpu.load_averages[1].toFixed(2)} | 15m: ${cpu.load_averages[2].toFixed(2)}`
    };

    let cpuHTML = '<div class="kv-grid">';
    for (const [k, v] of Object.entries(cpuKV)) {
      cpuHTML += `<div class="kv-item"><span class="kv-label">${k}</span><span class="kv-value">${this.escapeHTML(String(v))}</span></div>`;
    }
    cpuHTML += '</div>';

    if (cpu.reliability_notes) {
      cpuHTML += `<div class="alert-box alert-info" style="margin-top:1rem;">${this.escapeHTML(cpu.reliability_notes)}</div>`;
    }
    document.getElementById('cpu-details-card').innerHTML = cpuHTML;

    // Per-CPU Utilization Bars
    if (cpu.per_cpu_utilization && cpu.per_cpu_utilization.length > 0) {
      let barsHTML = '<div style="display:grid; grid-template-columns: repeat(auto-fill, minmax(130px, 1fr)); gap: 0.6rem; margin-top: 1rem;">';
      cpu.per_cpu_utilization.forEach((pct, idx) => {
        barsHTML += `
          <div style="background:var(--bg-tertiary); padding:0.5rem; border-radius:var(--radius-sm); font-size:0.75rem;">
            <div style="display:flex; justify-content:space-between; margin-bottom:0.25rem;">
              <span>CPU ${idx}</span><strong>${pct.toFixed(0)}%</strong>
            </div>
            <div class="progress-bar-bg"><div class="progress-bar-fill fill-brand" style="width:${Math.min(pct, 100)}%;"></div></div>
          </div>
        `;
      });
      barsHTML += '</div>';
      document.getElementById('cpu-cores-container').innerHTML = barsHTML;
    }

    // Memory Section (Linux Semantics)
    const memKV = {
      'Total Installed RAM': this.formatBytes(mem.total_bytes),
      'Available Memory': this.formatBytes(mem.available_bytes),
      'Used Memory': `${this.formatBytes(mem.used_bytes)} (${mem.utilization_pct.toFixed(1)}%)`,
      'Free Memory': this.formatBytes(mem.free_bytes),
      'Buffers': this.formatBytes(mem.buffers_bytes),
      'Cached & Reclaimable': this.formatBytes(mem.cached_bytes),
      'Swap Total': this.formatBytes(mem.swap_total_bytes),
      'Swap Used': `${this.formatBytes(mem.swap_used_bytes)} (${mem.swap_utilization_pct.toFixed(1)}%)`
    };

    let memHTML = '<div class="kv-grid">';
    for (const [k, v] of Object.entries(memKV)) {
      memHTML += `<div class="kv-item"><span class="kv-label">${k}</span><span class="kv-value">${this.escapeHTML(String(v))}</span></div>`;
    }
    memHTML += '</div>';
    document.getElementById('mem-details-card').innerHTML = memHTML;
  }

  // 4. Storage and Disk Health View
  static renderStorage(stg, smt) {
    if (!stg) return;

    // Physical Disks Table
    let disksHTML = `
      <div class="table-responsive">
        <table class="data-table">
          <thead>
            <tr>
              <th>Device</th>
              <th>Type</th>
              <th>Capacity</th>
              <th>Model</th>
              <th>Serial Number</th>
              <th>Transport</th>
              <th>Partitions</th>
              <th>State</th>
            </tr>
          </thead>
          <tbody>
    `;

    (stg.disks || []).forEach(d => {
      const typeBadge = d.device_type.includes('NVMe') ? 'badge-info' : (d.rotational ? 'badge-neutral' : 'badge-success');
      disksHTML += `
        <tr>
          <td><span class="code-pill">${this.escapeHTML(d.name)}</span></td>
          <td><span class="badge ${typeBadge}">${this.escapeHTML(d.device_type)}</span></td>
          <td><strong>${this.escapeHTML(d.size_formatted)}</strong></td>
          <td>${this.escapeHTML(d.model || '—')}</td>
          <td><span class="code-pill">${this.escapeHTML(d.serial_number || 'N/A')}</span></td>
          <td>${this.escapeHTML(d.transport || 'sata')}</td>
          <td>${d.partitions_count}</td>
          <td>${d.read_only ? '<span class="badge badge-warning">Read-Only</span>' : '<span class="badge badge-neutral">RW</span>'}</td>
        </tr>
      `;
    });
    disksHTML += '</tbody></table></div>';
    document.getElementById('storage-disks-container').innerHTML = disksHTML;

    // Filesystems Table
    let fsHTML = `
      <div class="table-responsive">
        <table class="data-table">
          <thead>
            <tr>
              <th>Mount Point</th>
              <th>Device</th>
              <th>FS Type</th>
              <th>Total Size</th>
              <th>Used</th>
              <th>Free</th>
              <th>Utilization</th>
              <th>Mount Status</th>
            </tr>
          </thead>
          <tbody>
    `;

    (stg.partitions || []).forEach(p => {
      let stateBadge = '<span class="badge badge-success">Normal</span>';
      if (p.warning_state === 'critical') stateBadge = '<span class="badge badge-danger">Critical (&gt;90%)</span>';
      else if (p.warning_state === 'warning') stateBadge = '<span class="badge badge-warning">Warning (&gt;80%)</span>';

      const roBadge = p.read_only ? '<span class="badge badge-danger">RO</span>' : '<span class="badge badge-neutral">RW</span>';

      fsHTML += `
        <tr>
          <td><strong>${this.escapeHTML(p.mount_point)}</strong></td>
          <td><span class="code-pill">${this.escapeHTML(p.device)}</span></td>
          <td>${this.escapeHTML(p.fs_type)}</td>
          <td>${this.formatBytes(p.total_bytes)}</td>
          <td>${this.formatBytes(p.used_bytes)}</td>
          <td>${this.formatBytes(p.free_bytes)}</td>
          <td style="min-width:140px;">
            <div style="display:flex; justify-content:space-between; font-size:0.75rem; margin-bottom:2px;">
              <span>${p.utilization_pct.toFixed(1)}%</span>
            </div>
            <div class="progress-bar-bg">
              <div class="progress-bar-fill ${p.warning_state === 'critical' ? 'fill-danger' : (p.warning_state === 'warning' ? 'fill-warning' : 'fill-brand')}"
                   style="width:${Math.min(p.utilization_pct, 100)}%;"></div>
            </div>
          </td>
          <td>${stateBadge} ${roBadge}</td>
        </tr>
      `;
    });
    fsHTML += '</tbody></table></div>';
    document.getElementById('storage-partitions-container').innerHTML = fsHTML;

    // SMART Cards
    if (smt) {
      let smartHTML = '<div class="grid-2">';
      (smt.devices || []).forEach(d => {
        let badge = 'badge-neutral';
        if (d.overall_status === 'healthy') badge = 'badge-success';
        else if (d.overall_status === 'warning') badge = 'badge-warning';
        else if (d.overall_status === 'critical') badge = 'badge-danger';

        smartHTML += `
          <div class="card">
            <div class="card-header">
              <span class="card-title">${this.escapeHTML(d.device)}</span>
              <span class="badge ${badge}">${this.escapeHTML(d.overall_status.toUpperCase())}</span>
            </div>
            <div class="kv-grid" style="font-size:0.8rem;">
              <div class="kv-item"><span class="kv-label">Model</span><span class="kv-value">${this.escapeHTML(d.model || 'Unknown')}</span></div>
              <div class="kv-item"><span class="kv-label">Temperature</span><span class="kv-value">${d.temperature_c !== undefined ? d.temperature_c + ' °C' : 'N/A'}</span></div>
              <div class="kv-item"><span class="kv-label">Power On Hours</span><span class="kv-value">${d.power_on_hours !== undefined ? d.power_on_hours + ' hrs' : 'N/A'}</span></div>
              <div class="kv-item"><span class="kv-label">Bad Sectors</span><span class="kv-value">${d.reallocated_sectors !== undefined ? d.reallocated_sectors : '0'}</span></div>
            </div>
            ${d.notes ? `<div style="font-size:0.75rem; color:var(--text-secondary); margin-top:0.5rem;">${this.escapeHTML(d.notes)}</div>` : ''}
          </div>
        `;
      });
      smartHTML += '</div>';
      document.getElementById('smart-container').innerHTML = smartHTML;
    }
  }

  // 5. GPU View
  static renderGPU(gpu) {
    if (!gpu) return;
    const container = document.getElementById('gpu-container');
    if (!gpu.present || !gpu.devices || gpu.devices.length === 0) {
      container.innerHTML = `
        <div class="card" style="text-align:center; padding:3rem 1rem;">
          <h3 style="color:var(--text-secondary); margin-bottom:0.5rem;">No Dedicated GPU Detected</h3>
          <p style="font-size:0.85rem; color:var(--text-muted);">
            No PCI display accelerators or NVIDIA graphics hardware were detected on this host. This is typical for headless servers and VMs.
          </p>
        </div>
      `;
      return;
    }

    let html = '<div class="grid-2">';
    gpu.devices.forEach(g => {
      html += `
        <div class="card">
          <div class="card-header">
            <span class="card-title">GPU #${g.index}: ${this.escapeHTML(g.model)}</span>
            <span class="badge badge-info">${this.escapeHTML(g.vendor)}</span>
          </div>
          <div class="kv-grid">
            <div class="kv-item"><span class="kv-label">PCI Bus</span><span class="kv-value">${this.escapeHTML(g.pci_bus || 'N/A')}</span></div>
            <div class="kv-item"><span class="kv-label">Kernel Driver</span><span class="kv-value">${this.escapeHTML(g.driver || 'N/A')} ${this.escapeHTML(g.driver_version || '')}</span></div>
            ${g.vram_total_bytes ? `<div class="kv-item"><span class="kv-label">VRAM Usage</span><span class="kv-value">${this.formatBytes(g.vram_used_bytes)} / ${this.formatBytes(g.vram_total_bytes)}</span></div>` : ''}
            ${g.utilization_pct !== undefined ? `<div class="kv-item"><span class="kv-label">GPU Core Util</span><span class="kv-value">${g.utilization_pct.toFixed(1)}%</span></div>` : ''}
            ${g.temperature_c ? `<div class="kv-item"><span class="kv-label">Core Temp</span><span class="kv-value">${g.temperature_c.toFixed(0)} °C</span></div>` : ''}
            ${g.power_watts ? `<div class="kv-item"><span class="kv-label">Power Draw</span><span class="kv-value">${g.power_watts.toFixed(1)} W</span></div>` : ''}
          </div>
        </div>
      `;
    });
    html += '</div>';
    container.innerHTML = html;
  }

  // 6. Network View
  static renderNetwork(net) {
    if (!net) return;
    document.getElementById('net-gateway').textContent = net.default_gateway || 'None';
    document.getElementById('net-dns').textContent = (net.dns_servers || []).join(', ') || 'None';

    let html = '<div class="grid-2">';
    (net.interfaces || []).forEach(iface => {
      const isUp = iface.state === 'up';
      const speedStr = iface.speed_mbps > 0 ? `${iface.speed_mbps} Mbps (${iface.duplex})` : 'Unknown / Virtual';

      html += `
        <div class="card">
          <div class="card-header">
            <div style="display:flex; align-items:center; gap:0.5rem;">
              <span class="dot ${isUp ? 'dot-success' : 'dot-danger'}"></span>
              <strong>${this.escapeHTML(iface.name)}</strong>
            </div>
            <span class="badge ${isUp ? 'badge-success' : 'badge-neutral'}">${this.escapeHTML(iface.state.toUpperCase())}</span>
          </div>
          <div class="kv-grid" style="font-size:0.82rem; margin-bottom:1rem;">
            <div class="kv-item"><span class="kv-label">MAC Address</span><span class="kv-value"><span class="code-pill">${this.escapeHTML(iface.mac || 'N/A')}</span></span></div>
            <div class="kv-item"><span class="kv-label">Link Speed</span><span class="kv-value">${this.escapeHTML(speedStr)}</span></div>
            <div class="kv-item"><span class="kv-label">MTU</span><span class="kv-value">${iface.mtu}</span></div>
            <div class="kv-item"><span class="kv-label">IPv4</span><span class="kv-value">${(iface.ipv4_addresses || ['—']).join(', ')}</span></div>
            <div class="kv-item"><span class="kv-label">IPv6</span><span class="kv-value">${(iface.ipv6_addresses || ['—']).join(', ')}</span></div>
          </div>
          <div style="background:var(--bg-tertiary); padding:0.6rem; border-radius:var(--radius-sm); font-size:0.75rem;">
            <div style="display:flex; justify-content:space-between; margin-bottom:0.25rem;">
              <span>RX: <strong>${this.formatBytes(iface.rx_bytes)}</strong> (${this.formatRate(iface.rx_rate_bytes_per_sec)})</span>
              <span>Errors / Drops: ${iface.rx_errors} / ${iface.rx_dropped}</span>
            </div>
            <div style="display:flex; justify-content:space-between;">
              <span>TX: <strong>${this.formatBytes(iface.tx_bytes)}</strong> (${this.formatRate(iface.tx_rate_bytes_per_sec)})</span>
              <span>Errors / Drops: ${iface.tx_errors} / ${iface.tx_dropped}</span>
            </div>
          </div>
        </div>
      `;
    });
    html += '</div>';
    document.getElementById('net-interfaces-container').innerHTML = html;
  }

  // 7. Services View
  static renderServices(srv, onFilterChange, onPageChange) {
    if (!srv) return;
    document.getElementById('srv-count-total').textContent = srv.total_services || 0;
    document.getElementById('srv-count-active').textContent = srv.active_services || 0;
    document.getElementById('srv-count-failed').textContent = srv.failed_services || 0;
    document.getElementById('srv-count-inactive').textContent = srv.inactive_services || 0;

    const tbody = document.getElementById('srv-table-body');
    if (!srv.services || srv.services.length === 0) {
      tbody.innerHTML = '<tr><td colspan="5" style="text-align:center; padding:2rem; color:var(--text-muted);">No services matching filter</td></tr>';
      return;
    }

    let rows = '';
    srv.services.forEach(s => {
      let stateBadge = '<span class="badge badge-success">active</span>';
      if (s.is_failed) stateBadge = '<span class="badge badge-danger">failed</span>';
      else if (s.active_state !== 'active') stateBadge = '<span class="badge badge-neutral">' + this.escapeHTML(s.active_state) + '</span>';

      rows += `
        <tr ${s.is_failed ? 'style="background:rgba(239, 68, 68, 0.05);"' : ''}>
          <td><strong>${this.escapeHTML(s.name)}</strong></td>
          <td>${this.escapeHTML(s.load_state)}</td>
          <td>${stateBadge}</td>
          <td><span class="code-pill">${this.escapeHTML(s.sub_state)}</span></td>
          <td>${this.escapeHTML(s.description || '—')}</td>
        </tr>
      `;
    });
    tbody.innerHTML = rows;

    // Pagination info
    document.getElementById('srv-page-info').textContent = `Page ${srv.page} of ${srv.total_pages || 1}`;
  }

  // 8. Health and Warnings View
  static renderHealth(hlt) {
    if (!hlt) return;
    const badgeEl = document.getElementById('hlt-overall-badge');
    if (badgeEl) {
      if (hlt.overall_status === 'healthy') badgeEl.innerHTML = '<span class="badge badge-success" style="font-size:1rem; padding:0.4rem 1rem;">HEALTHY</span>';
      else if (hlt.overall_status === 'warning') badgeEl.innerHTML = '<span class="badge badge-warning" style="font-size:1rem; padding:0.4rem 1rem;">WARNING</span>';
      else badgeEl.innerHTML = '<span class="badge badge-danger" style="font-size:1rem; padding:0.4rem 1rem;">CRITICAL</span>';
    }

    const issuesContainer = document.getElementById('hlt-issues-list');
    if (!hlt.issues || hlt.issues.length === 0) {
      issuesContainer.innerHTML = '<div class="card" style="text-align:center; padding:2rem; color:var(--color-success);">✓ All system subsystems, thresholds, and health checks are normal.</div>';
    } else {
      let html = '';
      hlt.issues.forEach(iss => {
        const cls = iss.severity === 'critical' ? 'alert-danger' : 'alert-warning';
        html += `
          <div class="alert-box ${cls}">
            <div style="flex:1;">
              <strong>[${this.escapeHTML(iss.component)}]</strong> ${this.escapeHTML(iss.message)}
              ${iss.details ? `<div style="font-size:0.8rem; margin-top:0.25rem; opacity:0.9;">${this.escapeHTML(iss.details)}</div>` : ''}
            </div>
            <span class="badge ${iss.severity === 'critical' ? 'badge-danger' : 'badge-warning'}">${iss.severity.toUpperCase()}</span>
          </div>
        `;
      });
      issuesContainer.innerHTML = html;
    }

    // Collector States
    const colContainer = document.getElementById('hlt-collectors-list');
    if (hlt.collector_state) {
      let colHTML = '<div class="grid-4">';
      for (const [col, state] of Object.entries(hlt.collector_state)) {
        const isOk = state === 'ok';
        colHTML += `
          <div class="card" style="padding:0.75rem;">
            <div style="display:flex; justify-content:space-between; align-items:center;">
              <span style="font-weight:600; text-transform:capitalize;">${col}</span>
              <span class="badge ${isOk ? 'badge-success' : 'badge-warning'}">${state}</span>
            </div>
          </div>
        `;
      }
      colHTML += '</div>';
      colContainer.innerHTML = colHTML;
    }
  }

  // 9. Reports View
  static renderReports(inv) {
    // Buttons for download are wired in app.js
  }

  // 10. Settings View
  static renderSettings(cfg, sess) {
    if (!cfg) return;
    const server = cfg.server || {};
    const auth = cfg.auth || {};

    document.getElementById('set-bind-addr').textContent = `${server.host}:${server.port}`;
    document.getElementById('set-remote-mode').textContent = server.allow_remote ? 'Remote Access Allowed' : 'Strict Localhost Only';
    document.getElementById('set-auth-status').textContent = auth.enabled ? 'Enabled' : 'Disabled (Local only)';
    document.getElementById('set-session-user').textContent = sess && sess.authenticated ? sess.username : 'Not Authenticated';
  }
}
