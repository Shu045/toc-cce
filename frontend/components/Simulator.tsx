"use client";
import { useEffect, useState } from "react";
import { getPresets, simulate, type Preset, type RunResult } from "@/lib/api";
import ResultView from "./ResultView";

export default function Simulator() {
  const [presets, setPresets] = useState<Preset[] | null>(null);
  const [loadErr, setLoadErr] = useState("");
  const [cur, setCur] = useState(0);
  const [n, setN] = useState("");
  const [lim, setLim] = useState("");
  const [res, setRes] = useState<RunResult | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  const load = () => {
    setLoadErr("");
    getPresets()
      .then((p) => {
        if (p.length === 0) throw new Error("The server returned no programs.");
        setPresets(p);
        setCur(0);
        setN(String(p[0].start));
        setLim(String(p[0].limit));
      })
      .catch((e: Error) => setLoadErr(e.message));
  };
  useEffect(load, []);

  const pick = (i: number) => {
    if (!presets) return;
    setCur(i);
    setN(String(presets[i].start));
    setLim(String(presets[i].limit));
    setRes(null);
    setErr("");
  };

  const run = async () => {
    if (!presets) return;
    setBusy(true);
    setErr("");
    try {
      const out = await simulate({ program: presets[cur].id, start: Number(n), limit: Number(lim) });
      setRes(out);
      if (out.limitAdjusted) setLim(String(out.limit));
    } catch (e) {
      setRes(null);
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  if (loadErr)
    return (
      <div className="res bad">
        <b>Couldn’t load the programs</b>
        {loadErr}
        <div><button className="go" onClick={load}>Try again</button></div>
      </div>
    );
  if (!presets) return <p className="sub">Loading programs…</p>;
  const p = presets[cur];

  return (
    <section>
      <p><b>Pick a program</b>, then press Run.</p>
      <div className="progs">
        {presets.map((q, i) => (
          <button key={q.id} className={"pg" + (i === cur ? " on" : "")} onClick={() => pick(i)}>
            <b>{q.name}</b><small>{q.sub}</small>
          </button>
        ))}
      </div>
      <div className="card">
        <div>{p.desc}</div>
        <pre>{p.code}</pre>
        <div className="row">
          <div><label>Start with x =</label><input type="number" min={0} value={n} onChange={(e) => setN(e.target.value)} /></div>
          <div><label>Give up after this many steps</label><input type="number" min={1} max={100000} value={lim} onChange={(e) => setLim(e.target.value)} /></div>
        </div>
        <button className="go" onClick={run} disabled={busy}>{busy ? "Running…" : "Run"}</button>
        {err && <div className="res bad"><b>Something went wrong</b>{err}</div>}
        {res && <ResultView r={res} mode="preset" />}
      </div>
    </section>
  );
}
