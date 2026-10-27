import {
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { SegmentedControl } from "../components/ui/segmented-control";
import type { RequestHistogram } from "./api";

export default function RequestChart({
  histogram,
  hours,
  onHoursChange,
  loading,
  error,
}: {
  histogram?: RequestHistogram;
  hours: string;
  onHoursChange: (hours: string) => void;
  loading: boolean;
  error: boolean;
}) {
  const buckets = histogram?.buckets || [];
  const label = (value: string) =>
    new Date(value).toLocaleString(undefined, {
      ...(hours === "168" ? { month: "numeric", day: "numeric" } : {}),
      hour: "2-digit",
      minute: "2-digit",
    });
  const interval = histogram?.interval_seconds || 0;
  return (
    <section
      className="request-chart"
      aria-label="请求量趋势"
      aria-describedby="request-chart-description"
      aria-busy={loading}
    >
      <div className="request-chart-heading">
        <div className="request-chart-title">
          <h2>请求量</h2>
          <span>
            {loading
              ? "正在加载…"
              : error
                ? "统计暂不可用"
                : histogram
                  ? `${histogram.total.toLocaleString()} 次请求 · 每 ${interval < 3600 ? `${interval / 60} 分钟` : `${interval / 3600} 小时`}`
                  : "统计暂不可用"}
          </span>
        </div>
        <SegmentedControl
          aria-label="统计时间范围"
          value={hours}
          onValueChange={onHoursChange}
          options={[
            { value: "1", label: "最近 1 小时" },
            { value: "24", label: "最近 24 小时" },
            { value: "168", label: "最近 7 天" },
          ]}
        />
      </div>
      <div className="request-chart-body">
        <ResponsiveContainer width="100%" height={110}>
          <BarChart
            data={error ? [] : buckets}
            accessibilityLayer
            aria-label="请求量趋势柱状图"
            margin={{ top: 4, right: 8, bottom: 0, left: 0 }}
            barCategoryGap="15%"
          >
            <CartesianGrid vertical={false} stroke="var(--line)" />
            <XAxis
              dataKey="started"
              tickFormatter={label}
              tick={{ fill: "var(--muted)", fontSize: 11 }}
              axisLine={false}
              tickLine={false}
              minTickGap={32}
              height={26}
            />
            <YAxis
              allowDecimals={false}
              tick={{ fill: "var(--muted)", fontSize: 11 }}
              tickFormatter={(value: number) => value.toLocaleString()}
              axisLine={false}
              tickLine={false}
              width={48}
              tickCount={3}
              domain={[0, (maximum: number) => Math.max(1, maximum)]}
            />
            <Tooltip
              cursor={{ fill: "var(--accent-soft)" }}
              labelFormatter={(value) => {
                const started = String(value);
                const end = new Date(
                  new Date(started).getTime() + interval * 1000,
                ).toISOString();
                return `${label(started)}–${label(end)}`;
              }}
              formatter={(value) => [
                `${Number(value).toLocaleString()} 次请求`,
                "请求量",
              ]}
              contentStyle={{
                background: "var(--surface)",
                border: "1px solid var(--line)",
                borderRadius: 6,
                fontSize: 12,
              }}
              labelStyle={{ color: "var(--muted)" }}
            />
            <Bar
              dataKey="count"
              name="请求量"
              fill="var(--accent)"
              radius={[2, 2, 0, 0]}
              isAnimationActive={false}
            />
          </BarChart>
        </ResponsiveContainer>
        {!loading && !error && histogram?.total === 0 && (
          <p className="request-chart-empty">这段时间没有匹配的请求</p>
        )}
        {(error || !histogram) && (
          <p className="request-chart-empty">
            {loading ? "正在加载请求量…" : "无法加载请求量，请刷新重试"}
          </p>
        )}
      </div>
      <p className="sr-only" id="request-chart-description">
        按请求开始时间统计，跟随搜索与状态筛选；下方列表保留全部历史记录。时间为本地时间。
      </p>
    </section>
  );
}
