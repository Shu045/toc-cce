"use client";
import { useState, type ReactNode } from "react";

export default function Story() {
  const [k, setK] = useState(1);
  const [said, setSaid] = useState<"s" | "l" | null>(null);

  const steps: ReactNode[] = [
    <><b>The question.</b> Someone hands you any program. Can you always figure out, before running it, whether it will eventually stop?</>,
    <><b>Running it only helps sometimes.</b> That’s what tab 1 does. If the program stops, great, you know. If it doesn’t, you just keep waiting and never get an answer.</>,
    <><b>Imagine a magic Checker.</b> You give it any program. It instantly says “stops” or “runs forever”, and it is never wrong.</>,
    <>
      <b>Now we build a trick program called Contrarian.</b> It asks the Checker about itself, then does the opposite:
      <pre>{'Contrarian:\n    if Checker says "Contrarian stops":\n        run forever\n    else:\n        stop immediately'}</pre>
    </>,
    <>
      <b>Ask the Checker about Contrarian.</b> What does it answer?
      <div className="two">
        <button className="sec" onClick={() => setSaid("s")}>Checker says: “stops”</button>
        <button className="sec" onClick={() => setSaid("l")}>Checker says: “runs forever”</button>
      </div>
      {said && (
        <>
          <div className="res bad">
            <b>The Checker is wrong.</b>
            {said === "s" ? "It said Contrarian stops, so Contrarian runs forever." : "It said Contrarian runs forever, so Contrarian stops right away."}{" "}
            Either answer is false. So a Checker that is always right cannot exist.
          </div>
          <p>This is the halting problem. The “we can’t tell” result in tab 1 isn’t a flaw in this tool. No tool can fully avoid it.</p>
        </>
      )}
    </>,
  ];

  return (
    <section>
      {steps.slice(0, k).map((s, i) => (
        <div className="card step" key={i}><span className="n">{i + 1}</span>{s}</div>
      ))}
      {k < steps.length && <button className="go" onClick={() => setK(k + 1)}>Next</button>}
    </section>
  );
}
