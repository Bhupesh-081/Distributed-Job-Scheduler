// Minimal dependency-free SVG sparkline/area chart.
export default function LineChart({ data, height = 72, color = "var(--accent)", fillOpacity = 0.18 }) {
  const w = 320;
  const h = height;

  if (!data || data.length < 2) {
    return <svg width="100%" height={h} viewBox={`0 0 ${w} ${h}`} className="chart-svg chart-empty" />;
  }

  const max = Math.max(...data, 0.001);
  const min = Math.min(...data, 0);
  const range = max - min || 1;
  const stepX = w / (data.length - 1);

  const points = data.map((v, i) => {
    const x = i * stepX;
    const y = h - ((v - min) / range) * (h - 8) - 4;
    return [x, y];
  });

  const linePath = points.map(([x, y], i) => `${i === 0 ? "M" : "L"}${x.toFixed(1)},${y.toFixed(1)}`).join(" ");
  const areaPath = `${linePath} L${w},${h} L0,${h} Z`;

  return (
    <svg width="100%" height={h} viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="none" className="chart-svg">
      <path d={areaPath} fill={color} opacity={fillOpacity} stroke="none" />
      <path d={linePath} fill="none" stroke={color} strokeWidth="2" strokeLinejoin="round" strokeLinecap="round" />
    </svg>
  );
}
