// CoreIP Server Inspector - Lightweight Chart Engine (Zero External Dependencies)

class CoreIPCharts {
  static setupCanvas(canvas) {
    const dpr = window.devicePixelRatio || 1;
    const rect = canvas.getBoundingClientRect();
    canvas.width = rect.width * dpr;
    canvas.height = rect.height * dpr;
    const ctx = canvas.getContext('2d');
    ctx.scale(dpr, dpr);
    return { ctx, width: rect.width, height: rect.height };
  }

  // Draw time-series line chart with filled gradient and axes
  static drawLineChart(canvasId, seriesList, options = {}) {
    const canvas = document.getElementById(canvasId);
    if (!canvas) return;

    const { ctx, width, height } = this.setupCanvas(canvas);
    ctx.clearRect(0, 0, width, height);

    const padding = { top: 15, right: 15, bottom: 25, left: 45 };
    const chartW = width - padding.left - padding.right;
    const chartH = height - padding.top - padding.bottom;

    if (chartW <= 0 || chartH <= 0) return;

    // Calculate overall min/max
    let maxVal = options.max !== undefined ? options.max : 0;
    let minVal = options.min !== undefined ? options.min : 0;
    let pointCount = 0;

    seriesList.forEach(s => {
      if (s.data && s.data.length > pointCount) pointCount = s.data.length;
      if (s.data) {
        s.data.forEach(v => {
          if (v > maxVal) maxVal = v;
          if (v < minVal) minVal = v;
        });
      }
    });

    if (maxVal === minVal) maxVal += 10;
    // Add 5% headroom
    maxVal *= 1.05;

    // Background horizontal grid lines
    ctx.strokeStyle = options.gridColor || 'rgba(255, 255, 255, 0.08)';
    ctx.lineWidth = 1;
    ctx.font = '10px -apple-system, sans-serif';
    ctx.fillStyle = options.labelColor || '#64748b';
    ctx.textAlign = 'right';

    const steps = 4;
    for (let i = 0; i <= steps; i++) {
      const yVal = minVal + ((maxVal - minVal) / steps) * i;
      const y = padding.top + chartH - (i / steps) * chartH;

      ctx.beginPath();
      ctx.moveTo(padding.left, y);
      ctx.lineTo(padding.left + chartW, y);
      ctx.stroke();

      // Label
      let label = yVal.toFixed(options.decimals !== undefined ? options.decimals : 0);
      if (options.unit) label += options.unit;
      ctx.fillText(label, padding.left - 8, y + 3);
    }

    if (pointCount < 2) {
      ctx.fillStyle = '#64748b';
      ctx.textAlign = 'center';
      ctx.fillText('Collecting metrics history...', width / 2, height / 2);
      return;
    }

    // Render each series
    seriesList.forEach(series => {
      const data = series.data || [];
      if (data.length < 2) return;

      const stepX = chartW / (data.length - 1);
      const points = data.map((val, idx) => {
        const x = padding.left + idx * stepX;
        const normalized = (val - minVal) / (maxVal - minVal);
        const y = padding.top + chartH - normalized * chartH;
        return { x, y };
      });

      // Gradient Fill
      if (series.fill) {
        const grad = ctx.createLinearGradient(0, padding.top, 0, padding.top + chartH);
        grad.addColorStop(0, series.fillColor || 'rgba(56, 189, 248, 0.25)');
        grad.addColorStop(1, 'rgba(56, 189, 248, 0.0)');

        ctx.beginPath();
        ctx.moveTo(points[0].x, padding.top + chartH);
        points.forEach(p => ctx.lineTo(p.x, p.y));
        ctx.lineTo(points[points.length - 1].x, padding.top + chartH);
        ctx.closePath();
        ctx.fillStyle = grad;
        ctx.fill();
      }

      // Stroke line
      ctx.beginPath();
      ctx.strokeStyle = series.color || '#38bdf8';
      ctx.lineWidth = series.lineWidth || 2;
      ctx.lineJoin = 'round';
      ctx.lineCap = 'round';

      ctx.moveTo(points[0].x, points[0].y);
      for (let i = 1; i < points.length; i++) {
        ctx.lineTo(points[i].x, points[i].y);
      }
      ctx.stroke();
    });
  }

  // Draw semi-gauge / ring indicator for percentage
  static drawRingGauge(canvasId, percentage, label, color = '#38bdf8') {
    const canvas = document.getElementById(canvasId);
    if (!canvas) return;

    const { ctx, width, height } = this.setupCanvas(canvas);
    ctx.clearRect(0, 0, width, height);

    const centerX = width / 2;
    const centerY = height / 2;
    const radius = Math.min(centerX, centerY) - 10;
    const lineWidth = 10;

    // Background ring
    ctx.beginPath();
    ctx.arc(centerX, centerY, radius, 0, 2 * Math.PI);
    ctx.strokeStyle = 'rgba(255, 255, 255, 0.08)';
    ctx.lineWidth = lineWidth;
    ctx.stroke();

    // Progress arc
    const pct = Math.min(Math.max(percentage, 0), 100);
    const startAngle = -0.5 * Math.PI;
    const endAngle = startAngle + (pct / 100) * 2 * Math.PI;

    ctx.beginPath();
    ctx.arc(centerX, centerY, radius, startAngle, endAngle);
    ctx.strokeStyle = color;
    ctx.lineWidth = lineWidth;
    ctx.lineCap = 'round';
    ctx.stroke();

    // Center text
    ctx.fillStyle = '#f8fafc';
    ctx.font = '700 16px -apple-system, sans-serif';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(`${pct.toFixed(1)}%`, centerX, centerY - 6);

    ctx.fillStyle = '#94a3b8';
    ctx.font = '500 10px -apple-system, sans-serif';
    ctx.fillText(label, centerX, centerY + 12);
  }
}
