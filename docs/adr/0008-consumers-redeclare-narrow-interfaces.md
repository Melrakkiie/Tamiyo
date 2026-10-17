# Each consumer redeclares the narrow interface it needs

A context that depends on another declares its own small interface instead of importing the other context's concrete type (`deckService` exists separately in `internal/bulk` and `internal/deckinsights`), which keeps contexts decoupled. The cost: changing a shared method's signature only breaks compilation where the redeclared interface was also changed, and can miss the others silently (it happened twice while adding the mana value sort). After such a change, grep the method name across the repo.
