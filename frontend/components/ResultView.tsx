import type { ReactNode } from "react";
import type { RunResult } from "@/lib/api";

function Box({ cls, b, children }: { cls: string; b: string; children: ReactNode }) {
  return (
    <div className={"res " + cls}>
      <b>{b}</b>
      {children}
    </div>
  );
}

export default function ResultView({ r, mode }: { r: RunResult; mode: "preset" | "code" }) {
  const py = mode === "code";
  let box: ReactNode;
  switch (r.verdict) {
    case "halts":
      box = py ? (
        <Box cls="ok" b="It stops.">
          {r.steps ? `The program finished after ${r.steps} loop iterations.` : "The loop condition was false right away, so the loop body never ran."} Final values: {r.final || "none"}.
        </Box>
      ) : (
        <Box cls="ok" b="It stops.">It finished after {r.steps} steps.</Box>
      );
      break;
    case "loops":
      box = py ? (
        <Box cls="bad" b="It loops forever.">
          The loop on line {r.loopLine} was reached again with exactly the same values ({r.state || "no variables"}), first seen at check #{r.firstSeen} and again at check #{r.repeatedAt}. Nothing here changes between visits, so it repeats the same steps forever.
        </Box>
      ) : (
        <Box cls="bad" b="It loops forever.">
          The value {r.state} showed up at step {r.firstSeen} and again at step {r.repeatedAt}. Same situation, same future, so it will repeat forever.
        </Box>
      );
      break;
    case "crash":
      box = <Box cls="warn" b="It stops by crashing.">{r.message || "Go would panic here."} A real Go program ends at that point.</Box>;
      break;
    default:
      box = (
        <Box cls="warn" b="We can’t tell.">
          I gave up after {r.limit} {py ? "loop iterations" : "steps"} and nothing repeated. It might run forever, or it might just be slow. Try a bigger step limit.
        </Box>
      );
  }
  const head = r.elided ? r.trail.slice(0, 12) : r.trail;
  const tail = r.elided ? r.trail.slice(12) : [];
  const trail = !r.trail.length ? "(none)" : head.join(" → ") + (r.elided ? ` → … (${r.elided} more) → ` + tail.join(" → ") : "");
  return (
    <>
      {box}
      {r.limitAdjusted && <p className="trail">Step limit adjusted to {r.limit} (allowed: 1 to 100,000).</p>}
      {py && r.notes && r.notes.length > 0 && <p className="trail">Quick check: {r.notes.join("  ")}</p>}
      <div className="trail">{py ? "Values at each loop check: " : "x went: "}{trail}</div>
    </>
  );
}
