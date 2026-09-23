export function formatRupiah(value: number) {
  return `Rp${Math.round(value / 1000)}k`;
}
