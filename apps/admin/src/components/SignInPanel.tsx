import { AlertTriangle, Coffee, LoaderCircle, Mail } from "lucide-react";
import { FormEvent, useState } from "react";

type Props = {
  signingIn: boolean;
  error: string;
  sentTo: string;
  onSignIn: (email: string) => Promise<void>;
  onUseAnotherAddress: () => void;
};

export function SignInPanel({ signingIn, error, sentTo, onSignIn, onUseAnotherAddress }: Props) {
  const [email, setEmail] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    const address = email.trim();
    if (!address || signingIn) return;
    await onSignIn(address);
  }

  return <main className="grid min-h-screen place-items-center bg-paper px-5 py-12 text-ink">
    <div className="w-full max-w-md rounded-2xl border border-ink/15 bg-[#f7f5ee] p-7 shadow-lift sm:p-8">
      <div className="flex items-center gap-3"><div className="grid h-10 w-10 place-items-center rounded-lg bg-moss text-white"><Coffee className="h-5 w-5" /></div><div className="leading-none"><p className="font-display text-[23px]">where to <i>WFC</i></p><p className="mt-1 text-[9px] font-bold uppercase tracking-[0.22em] text-ink/45">Catalogue admin</p></div></div>
      {sentTo ? <>
        <h1 className="mt-8 font-display text-[32px] leading-none">Check your <i>inbox.</i></h1>
        <p className="mt-3 text-sm leading-6 text-ink/60">We sent a one-time sign-in link to <span className="font-semibold text-ink">{sentTo}</span>. It only works once, so open it in this browser.</p>
        <button type="button" onClick={onUseAnotherAddress} className="focus-ring mt-6 inline-flex h-10 items-center rounded-lg border border-ink/15 px-3 text-xs font-medium transition hover:bg-cream">Use a different address</button>
      </> : <>
        <h1 className="mt-8 font-display text-[32px] leading-none">Sign in to <i>review sources.</i></h1>
        <p className="mt-3 text-sm leading-6 text-ink/60">Only invited contributors can sign in. We email a one-time link — there is no password to remember.</p>
        <form onSubmit={submit} className="mt-6 space-y-3">
          <label className="block"><span className="sr-only">Email address</span><span className="relative block"><Mail className="absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-ink/35" /><input type="email" required autoFocus value={email} onChange={(event) => setEmail(event.target.value)} placeholder="you@example.com" className="focus-ring h-11 w-full rounded-lg border border-ink/20 bg-white pl-10 pr-3 text-sm outline-none placeholder:text-ink/35" /></span></label>
          <button type="submit" disabled={signingIn} className="focus-ring inline-flex h-11 w-full items-center justify-center gap-2 rounded-lg bg-moss px-4 text-sm font-semibold text-white transition hover:bg-[#203d2b] disabled:opacity-60">{signingIn ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Mail className="h-4 w-4" />} {signingIn ? "Sending the link…" : "Email me a sign-in link"}</button>
        </form>
        {error ? <p className="mt-4 flex items-start gap-2 text-xs leading-5 text-red-800"><AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" /> {error}</p> : null}
      </>}
    </div>
  </main>;
}
