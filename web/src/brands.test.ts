import { describe, expect, it } from "vitest";
import type { Account } from "./api";
import { ALL_BRANDS, brandsOf, matchesBrand, resolveFilter } from "./brands";

function account(id: string, brand?: string): Account {
  return {
    id,
    platform: "discord",
    display_name: id,
    created_at: "2026-11-14T00:00:00Z",
    brand,
  };
}

const accounts = [account("1", "norviq"), account("2", "Aaru"), account("3"), account("4", "Aaru")];

describe("brands", () => {
  it("lists each brand once, alphabetically whatever its case", () => {
    expect(brandsOf(accounts)).toEqual(["Aaru", "norviq"]);
    expect(brandsOf([account("1")])).toEqual([]);
  });

  it("shows every account under All, and only its own under a brand", () => {
    expect(accounts.filter((a) => matchesBrand(a, ALL_BRANDS)).map((a) => a.id)).toEqual(["1", "2", "3", "4"]);
    expect(accounts.filter((a) => matchesBrand(a, { kind: "brand", brand: "Aaru" })).map((a) => a.id)).toEqual([
      "2",
      "4",
    ]);
  });

  it("falls back to All when the chosen brand is gone", () => {
    expect(resolveFilter(accounts, { kind: "brand", brand: "Loci" })).toEqual(ALL_BRANDS);
    expect(resolveFilter(accounts, { kind: "brand", brand: "Aaru" })).toEqual({
      kind: "brand",
      brand: "Aaru",
    });
  });
});
