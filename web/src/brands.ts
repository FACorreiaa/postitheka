import type { Account } from "./api";

/** Which accounts the composer shows: every one, or one brand's. */
export type BrandFilter = { kind: "all" } | { kind: "brand"; brand: string };

export const ALL_BRANDS: BrandFilter = { kind: "all" };

/** The brands in use, each once, in alphabetical order whatever their case. */
export function brandsOf(accounts: Account[]): string[] {
  const seen = new Set<string>();
  for (const a of accounts) if (a.brand) seen.add(a.brand);
  // oxlint-disable-next-line unicorn/no-array-sort -- sorts the copy made here; toSorted() is ES2023, lib is ES2022
  return [...seen].sort((a, b) => a.localeCompare(b, undefined, { sensitivity: "base" }));
}

export function matchesBrand(account: Account, filter: BrandFilter): boolean {
  return filter.kind === "all" || account.brand === filter.brand;
}

/**
 * The filter to show, given the one last chosen. A brand that no account
 * carries any more (renamed, or its last account disconnected) falls back to
 * every account, so the composer is never left showing nothing.
 */
export function resolveFilter(accounts: Account[], filter: BrandFilter): BrandFilter {
  if (filter.kind === "brand" && !accounts.some((a) => a.brand === filter.brand)) return ALL_BRANDS;
  return filter;
}
