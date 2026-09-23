import type { TitlePart } from "../i18n";

// The accented run is one phrase, so it must not break across lines: an italic
// fragment split mid-phrase reads as a mistake rather than emphasis.
export function Title({ parts }: { parts: TitlePart[] }) {
  return <>{parts.map((part, index) => part.accent ? <i key={index} className="whitespace-nowrap">{part.text}</i> : <span key={index}>{part.text}</span>)}</>;
}
