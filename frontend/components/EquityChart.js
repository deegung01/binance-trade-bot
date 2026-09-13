"use client";

import { useEffect, useRef } from "react";
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";

export default function EquityChart({ points }) {
  const data = (points || []).map((p) => ({
    ts: p.ts,
    time: new Date(p.ts).getTime(),
    equity: Number(p.equity.toFixed(2)),
  }));

  if (data.length < 2) {
    return (
      <div className="h-56 flex items-center justify-center text-sm text-zinc-500">
        Collecting data — needs ≥ 2 engine cycles
      </div>
    );
  }

  const first = data[0].equity;
  const last = data[data.length - 1].equity;
  const color = last >= first ? "#34d399" : "#fb7185";
  const stroke = last >= first ? "#34d399" : "#fb7185";

  return (
    <div className="h-56">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={data} margin={{ top: 4, right: 4, bottom: 0, left: 4 }}>
          <defs>
            <linearGradient id="eq" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={stroke} stopOpacity={0.3} />
              <stop offset="100%" stopColor={stroke} stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid stroke="#1e2430" strokeDasharray="3 3" vertical={false} />
          <XAxis
            dataKey="time"
            type="number"
            scale="time"
            domain={["dataMin", "dataMax"]}
            tickFormatter={(t) =>
              new Date(t).toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" })
            }
            stroke="#3f4757"
            tick={{ fill: "#71767f", fontSize: 10 }}
            tickLine={false}
            axisLine={{ stroke: "#1e2430" }}
          />
          <YAxis
            domain={["auto", "auto"]}
            tickFormatter={(v) => `$${v}`}
            stroke="#3f4757"
            tick={{ fill: "#71767f", fontSize: 10 }}
            tickLine={false}
            axisLine={false}
            width={56}
          />
          <Tooltip
            contentStyle={{
              background: "#11151d",
              border: "1px solid #1e2430",
              borderRadius: 8,
              fontSize: 12,
            }}
            labelFormatter={(t) => new Date(t).toLocaleTimeString()}
            formatter={(v) => [`$${Number(v).toFixed(2)}`, "Equity"]}
          />
          <Area type="monotone" dataKey="equity" stroke={color} strokeWidth={1.6} fill="url(#eq)" />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}
