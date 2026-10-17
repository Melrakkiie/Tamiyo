# Scryfall failures are best-effort for ManaBox imports, fatal for Moxfield and lists

The asymmetry is deliberate. A ManaBox CSV carries its own `scryfall_id`, so the import worked without Scryfall; the later call that fetches `mana_value` and colors is best-effort, and the import goes through with zeros when Scryfall is down. Moxfield and text-list imports must resolve set + collector number to a `scryfall_id` through Scryfall, so they cannot succeed without it and fail instead.
