"use client";
import { useRef, useState, type KeyboardEvent } from "react";
import { checkCode, type RunResult } from "@/lib/api";
import ResultView from "./ResultView";

export default function LoopChecker() {
  const [src, setSrc] = useState("");
  const [lim, setLim] = useState("10000");
  const [res, setRes] = useState<RunResult | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const tabFree = useRef(false);

  const check = async () => {
    if (busy) return;
    setBusy(true);
    setErr("");
    try {
      const out = await checkCode({ code: src, limit: Number(lim) });
      setRes(out);
      if (out.limitAdjusted) setLim(String(out.limit));
    } catch (e) {
      setRes(null);
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const onKey = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Escape") { tabFree.current = true; return; }
    if ((e.ctrlKey || e.metaKey) && e.key === "Enter") { e.preventDefault(); void check(); return; }
    if (e.key !== "Tab" || e.ctrlKey || e.metaKey || e.altKey) return;
    if (tabFree.current) { tabFree.current = false; return; }
    e.preventDefault();
    const el = e.currentTarget, v = el.value, a = el.selectionStart, b = el.selectionEnd;
    const apply = (text: string, s: number, en: number, selA: number, selB: number) => {
      el.setRangeText(text, s, en, "end");
      setSrc(el.value);
      requestAnimationFrame(() => el.setSelectionRange(selA, selB));
    };
    if (a === b && !e.shiftKey) { apply("\t", a, b, a + 1, a + 1); return; }
    const ls = v.lastIndexOf("\n", a - 1) + 1;
    let le = v.indexOf("\n", b > a && v[b - 1] === "\n" ? b - 1 : b);
    if (le < 0) le = v.length;
    const old = v.slice(ls, le);
    const nt = old.split("\n").map((l) => (e.shiftKey ? l.replace(/^(\t| {1,4})/, "") : "\t" + l)).join("\n");
    if (a === b) { const q = Math.max(ls, a + nt.length - old.length); apply(nt, ls, le, q, q); }
    else apply(nt, ls, le, ls, ls + nt.length);
  };

  return (
    <section>
      <p><b>Write or paste Go code with a loop.</b> The Go server parses it with go/parser, runs it safely under a step limit, and tells you whether it stops.</p>
      <div className="card">
        <textarea
          value={src} onChange={(e) => setSrc(e.target.value)} onKeyDown={onKey}
          placeholder="Write Go here. Statements alone are fine, or a full program with package main and func main()."
          spellCheck={false} autoComplete="off" autoCapitalize="off" autoCorrect="off"
        />
        <div className="row">
          <div><label>Give up after this many steps</label><input type="number" min={1} max={100000} value={lim} onChange={(e) => setLim(e.target.value)} /></div>
        </div>
        <button className="go" onClick={() => void check()} disabled={busy}>{busy ? "Checking…" : "Check it"}</button>
        {err && <div className="res bad"><b>Let’s fix this first</b>{err}</div>}
        {res && <ResultView r={res} mode="code" />}
        <p className="trail">
          Supported Go: variables (int, int8–int64, uint8–uint32, float64, bool) with real Go overflow wrap-around, := and var,
          arithmetic, comparisons, &&, ||, !, bit operators and shifts, if / else, for (all forms, including range over an integer),
          break / continue / return, min, max, number conversions, and math.Abs / Sqrt / Floor / Ceil / Pow / Mod. fmt.Print* is ignored.
          Not supported: functions other than main, slices, maps, strings, structs, pointers, goroutines, switch, imports other than fmt and math.
          Press Tab to indent, Shift+Tab to un-indent (Esc then Tab leaves the box). Ctrl+Enter runs it.
        </p>
      </div>
    </section>
  );
}
