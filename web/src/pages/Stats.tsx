import { useEffect, useRef, useState } from 'react';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from 'recharts';
import { metricsApi } from '../api/client';
import { useSSE } from '../hooks/useSSE';
import { useFsStore } from '../store/fsStore';
import type { MetricsSnapshot, SSEEvent } from '../types';

interface DataPoint extends MetricsSnapshot {
  time: string;
}

const MAX_POINTS = 60;

function StatCard({ title, value, unit, color }: { title: string; value: string | number; unit?: string; color: string }) {
  return (
    <div className="bg-[#161b22] border border-[#30363d] rounded-lg p-4">
      <div className="text-xs text-[#8b949e] uppercase tracking-wider font-semibold mb-1">{title}</div>
      <div className="text-2xl font-bold font-mono" style={{ color }}>
        {value}<span className="text-sm text-[#8b949e] ml-1">{unit}</span>
      </div>
    </div>
  );
}

const chartTheme = {
  gridColor: '#21262d',
  axisColor: '#484f58',
  tooltipBg: '#161b22',
  tooltipBorder: '#30363d',
};

function ChartFrame({ title, renderChart }: { title: string; renderChart: (size: { width: number; height: number }) => React.ReactNode }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState({ width: 0, height: 0 });

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;

    const update = () => {
      setSize({ width: el.clientWidth, height: el.clientHeight });
    };

    update();
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  return (
    <div className="bg-[#161b22] border border-[#30363d] rounded-lg p-4 flex flex-col">
      <div className="text-xs text-[#8b949e] uppercase tracking-wider font-semibold mb-3">{title}</div>
      <div ref={containerRef} className="flex-1 min-h-[240px] min-w-0">
        {size.width > 0 && size.height > 0 ? renderChart(size) : null}
      </div>
    </div>
  );
}

export default function Stats() {
  const [data, setData] = useState<DataPoint[]>([]);
  const [latest, setLatest] = useState<MetricsSnapshot | null>(null);
  const { setMetrics, handleSSEEvent } = useFsStore();
  const tickRef = useRef(0);

  const addPoint = (m: MetricsSnapshot) => {
    const now = new Date();
    const time = now.toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });
    setData(prev => [...prev.slice(-(MAX_POINTS - 1)), { ...m, time }]);
    setLatest(m);
    setMetrics(m);
  };

  const fetchMetrics = async () => {
    try {
      const resp = await metricsApi.get() as { counters?: Record<string, number>; hitRate?: number };
      const c = resp.counters ?? {};
      const hits = c.cache_hits ?? 0;
      const misses = c.cache_miss ?? 0;
      const total = hits + misses;
      const m: MetricsSnapshot = {
        reads: c.reads ?? 0,
        writes: c.writes ?? 0,
        cacheHits: hits,
        cacheMisses: misses,
        journalCommits: c.journal_commits ?? 0,
        blocksAllocated: c.blocks_allocated ?? 0,
        blocksFreed: c.blocks_freed ?? 0,
        inodesAllocated: c.inodes_allocated ?? 0,
        inodesFreed: c.inodes_freed ?? 0,
        hitRate: resp.hitRate ?? (total > 0 ? hits / total : 0),
      };
      addPoint(m);
    } catch {
      // ignore
    }
  };

  useSSE('/sse', (event: SSEEvent) => {
    handleSSEEvent(event);
    tickRef.current++;
    if (tickRef.current % 5 === 0) {
      fetchMetrics();
    }
  });

  useEffect(() => {
    fetchMetrics();
    const interval = setInterval(fetchMetrics, 3000);
    return () => clearInterval(interval);
  }, []);

  const CustomTooltip = ({ active, payload, label }: { active?: boolean; payload?: Array<{ color: string; name: string; value: number }>; label?: string }) => {
    if (!active || !payload?.length) return null;
    return (
      <div className="rounded border px-3 py-2 text-xs" style={{ backgroundColor: chartTheme.tooltipBg, borderColor: chartTheme.tooltipBorder }}>
        <div className="text-[#8b949e] mb-1">{label}</div>
        {payload.map((p) => (
          <div key={p.name} className="flex gap-2">
            <span style={{ color: p.color }}>{p.name}:</span>
            <span className="text-[#c9d1d9] font-mono">{p.value}</span>
          </div>
        ))}
      </div>
    );
  };

  return (
    <div className="flex flex-col h-full gap-4 p-4 overflow-auto">
      {/* Summary cards */}
      <div className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-5 gap-3">
        <StatCard title="Cache Hit Rate" value={latest ? `${(latest.hitRate * 100).toFixed(1)}` : '—'} unit="%" color="#3fb950" />
        <StatCard title="Total Reads" value={latest?.reads ?? '—'} color="#79c0ff" />
        <StatCard title="Total Writes" value={latest?.writes ?? '—'} color="#388bfd" />
        <StatCard title="Journal Commits" value={latest?.journalCommits ?? '—'} color="#a371f7" />
        <StatCard title="Blocks Free" value={latest ? latest.blocksFreed - latest.blocksAllocated : '—'} color="#e3b341" />
      </div>

      {/* Charts */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 flex-1">
        <ChartFrame title="I/O Operations (cumulative)" renderChart={({ width, height }) => (
          <ResponsiveContainer width={width} height={height}>
            <LineChart data={data} margin={{ top: 5, right: 10, bottom: 5, left: 10 }}>
              <CartesianGrid stroke={chartTheme.gridColor} strokeDasharray="3 3" />
              <XAxis dataKey="time" tick={{ fill: chartTheme.axisColor, fontSize: 10 }} tickLine={false} interval="preserveStartEnd" />
              <YAxis tick={{ fill: chartTheme.axisColor, fontSize: 10 }} tickLine={false} axisLine={false} />
              <Tooltip content={<CustomTooltip />} />
              <Legend wrapperStyle={{ fontSize: '11px', color: '#8b949e' }} />
              <Line type="monotone" dataKey="reads" stroke="#79c0ff" strokeWidth={2} dot={false} name="Reads" />
              <Line type="monotone" dataKey="writes" stroke="#388bfd" strokeWidth={2} dot={false} name="Writes" />
            </LineChart>
          </ResponsiveContainer>
        )} />

        <ChartFrame title="Cache Performance" renderChart={({ width, height }) => (
          <ResponsiveContainer width={width} height={height}>
            <LineChart data={data} margin={{ top: 5, right: 10, bottom: 5, left: 10 }}>
              <CartesianGrid stroke={chartTheme.gridColor} strokeDasharray="3 3" />
              <XAxis dataKey="time" tick={{ fill: chartTheme.axisColor, fontSize: 10 }} tickLine={false} interval="preserveStartEnd" />
              <YAxis tick={{ fill: chartTheme.axisColor, fontSize: 10 }} tickLine={false} axisLine={false} />
              <Tooltip content={<CustomTooltip />} />
              <Legend wrapperStyle={{ fontSize: '11px', color: '#8b949e' }} />
              <Line type="monotone" dataKey="cacheHits" stroke="#3fb950" strokeWidth={2} dot={false} name="Cache Hits" />
              <Line type="monotone" dataKey="cacheMisses" stroke="#f85149" strokeWidth={2} dot={false} name="Cache Misses" />
            </LineChart>
          </ResponsiveContainer>
        )} />

        <ChartFrame title="Block Allocation" renderChart={({ width, height }) => (
          <ResponsiveContainer width={width} height={height}>
            <LineChart data={data} margin={{ top: 5, right: 10, bottom: 5, left: 10 }}>
              <CartesianGrid stroke={chartTheme.gridColor} strokeDasharray="3 3" />
              <XAxis dataKey="time" tick={{ fill: chartTheme.axisColor, fontSize: 10 }} tickLine={false} interval="preserveStartEnd" />
              <YAxis tick={{ fill: chartTheme.axisColor, fontSize: 10 }} tickLine={false} axisLine={false} />
              <Tooltip content={<CustomTooltip />} />
              <Legend wrapperStyle={{ fontSize: '11px', color: '#8b949e' }} />
              <Line type="monotone" dataKey="blocksAllocated" stroke="#e3b341" strokeWidth={2} dot={false} name="Allocated" />
              <Line type="monotone" dataKey="blocksFreed" stroke="#ffa657" strokeWidth={2} dot={false} name="Freed" />
            </LineChart>
          </ResponsiveContainer>
        )} />

        <ChartFrame title="Inode Operations & Journal" renderChart={({ width, height }) => (
          <ResponsiveContainer width={width} height={height}>
            <LineChart data={data} margin={{ top: 5, right: 10, bottom: 5, left: 10 }}>
              <CartesianGrid stroke={chartTheme.gridColor} strokeDasharray="3 3" />
              <XAxis dataKey="time" tick={{ fill: chartTheme.axisColor, fontSize: 10 }} tickLine={false} interval="preserveStartEnd" />
              <YAxis tick={{ fill: chartTheme.axisColor, fontSize: 10 }} tickLine={false} axisLine={false} />
              <Tooltip content={<CustomTooltip />} />
              <Legend wrapperStyle={{ fontSize: '11px', color: '#8b949e' }} />
              <Line type="monotone" dataKey="inodesAllocated" stroke="#a371f7" strokeWidth={2} dot={false} name="Inodes Alloc" />
              <Line type="monotone" dataKey="inodesFreed" stroke="#d2a8ff" strokeWidth={2} dot={false} name="Inodes Freed" />
              <Line type="monotone" dataKey="journalCommits" stroke="#388bfd" strokeWidth={2} dot={false} name="Journal Commits" />
            </LineChart>
          </ResponsiveContainer>
        )} />
      </div>

      {/* Hit rate gauge */}
      {latest && (
        <div className="bg-[#161b22] border border-[#30363d] rounded-lg p-4">
          <div className="text-xs text-[#8b949e] uppercase tracking-wider font-semibold mb-2">Cache Hit Rate</div>
          <div className="flex items-center gap-3">
            <div className="flex-1 bg-[#0d1117] rounded-full h-3 overflow-hidden border border-[#30363d]">
              <div
                className="h-full rounded-full transition-all duration-500"
                style={{
                  width: `${(latest.hitRate * 100).toFixed(1)}%`,
                  backgroundColor: latest.hitRate > 0.8 ? '#3fb950' : latest.hitRate > 0.5 ? '#e3b341' : '#f85149',
                }}
              />
            </div>
            <span className="text-sm font-mono font-bold" style={{
              color: latest.hitRate > 0.8 ? '#3fb950' : latest.hitRate > 0.5 ? '#e3b341' : '#f85149'
            }}>
              {(latest.hitRate * 100).toFixed(1)}%
            </span>
            <span className="text-xs text-[#484f58]">
              {latest.cacheHits} hits / {latest.cacheMisses} misses
            </span>
          </div>
        </div>
      )}
    </div>
  );
}
