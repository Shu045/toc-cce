"use client";
import { useState } from "react";
import Simulator from "@/components/Simulator";
import Story from "@/components/Story";
import LoopChecker from "@/components/LoopChecker";

const TABS = ["1. Try it", "2. Why we can’t always know", "3. Your own loop"];

export default function Page() {
  const [t, setT] = useState(0);
  return (
    <main>
      <h1>Will it ever stop?</h1>
      <p className="sub">Run a small program and see if it stops, repeats forever, or if we simply can’t tell.</p>
      <div className="tabs">
        {TABS.map((n, i) => (
          <button key={n} className={i === t ? "on" : ""} onClick={() => setT(i)}>{n}</button>
        ))}
      </div>
      {/* Panels stay mounted so typed code isn’t lost when switching tabs */}
      <div className={t === 0 ? "" : "hide"}><Simulator /></div>
      <div className={t === 1 ? "" : "hide"}><Story /></div>
      <div className={t === 2 ? "" : "hide"}><LoopChecker /></div>
    </main>
  );
}
