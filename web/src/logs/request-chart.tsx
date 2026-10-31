import { getLocale, t, useLocale } from "../i18n";
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
  useLocale();
  const buckets = histogram?.buckets || [];
  const label = (value: string) =>
    new Date(value).toLocaleString(getLocale(), {
      ...(hours === "168" ? { month: "numeric", day: "numeric" } : {}),
      hour: "2-digit",
      minute: "2-digit",
    });
  const interval = histogram?.interval_seconds || 0;
  return (
    <section
      className="request-chart"
      aria-label={t("Request volume trend")}
      aria-describedby="request-chart-description"
      aria-busy={loading}
    >
      <div className="request-chart-heading">
        <div className="request-chart-title">
          <h2>{t("Requests")}</h2>
          <span>
            {loading
              ? t("Loading…")
              : error
                ? t("Statistics unavailable")
                : histogram
                  ? t("{0} requests · per {1}", [
                      histogram.total.toLocaleString(getLocale()),
                      interval < 3600
                        ? t("{0} min", [interval / 60])
                        : t("{0} hr", [interval / 3600]),
                    ])
                  : t("Statistics unavailable")}
          </span>
        </div>
        <SegmentedControl
          aria-label={t("Statistics time range")}
          value={hours}
          onValueChange={onHoursChange}
          options={[
            { value: "1", label: t("Last hour") },
            { value: "24", label: t("Last 24 hours") },
            { value: "168", label: t("Last 7 days") },
          ]}
        />
      </div>
      <div className="request-chart-body">
        <ResponsiveContainer width="100%" height={110}>
          <BarChart
            data={error ? [] : buckets}
            accessibilityLayer
            aria-label={t("Request volume bar chart")}
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
              tickFormatter={(value: number) =>
                value.toLocaleString(getLocale())
              }
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
                t("{0} requests", [Number(value).toLocaleString(getLocale())]),
                t("Requests"),
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
              name={t("Requests")}
              fill="var(--accent)"
              radius={[2, 2, 0, 0]}
              isAnimationActive={false}
            />
          </BarChart>
        </ResponsiveContainer>
        {!loading && !error && histogram?.total === 0 && (
          <p className="request-chart-empty">
            {t("No matching requests in this period")}
          </p>
        )}
        {(error || !histogram) && (
          <p className="request-chart-empty">
            {loading
              ? t("Loading request volume…")
              : t("Could not load request volume. Refresh to try again.")}
          </p>
        )}
      </div>
      <p className="sr-only" id="request-chart-description">
        {t(
          "Grouped by request start time and filtered by search and status. The list below retains all history. Times are local.",
        )}
      </p>
    </section>
  );
}
