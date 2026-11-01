# Gift: Architektur- und Implementierungsplan

Status: Entwurf zum Review, Revision 2, 2026-09-13. Noch keine implementierte API.

Dieses Dokument beschreibt den ersten vertikalen Prototyp und seine Grenzen.
Codebeispiele sind API-Skizzen, keine bereits kompilierten Zusagen. Erst nach
Review beginnt die Implementierung.

Modulpfad: `github.com/worldiety/gift`. Toolchain: Go 1.27 (generische
Methoden), Ebitengine 2.10.1. Die go.mod ist bereits entsprechend gesetzt;
Ebitengine wird in Schritt 1 gepinnt.

## 1. Festgelegte Ziele

- SwiftUI-artige, deklarative Go-API nach Variante A: konkrete Value-Views und
  fluent Modifier, heterogene Kinderlisten ueber eine schmale View-Schnittstelle.
- Generics und generische Methoden dort, wo sie Typsicherheit oder konkrete
  Speicherung verbessern. Keine Reflection im regulaeren Layout-/Renderpfad.
- Rendererunabhaengige Deklaration, gemeinsames Layout und ein CGO-freies
  Ebitengine-Desktop-Backend.
- Zielplattform: Raspberry Pi 4 und Pi 5, Raspberry Pi OS 64-Bit,
  X11/XWayland, hardwarebeschleunigtes Mesa, 1920x1080 bei 60 Hz.
  Desktop-Ziele daneben: macOS und Linux; Windows 11 als Best-Effort-Ziel
  seit Abschnitt 43, geprueft auf einem Windows-PC des Nutzers, nicht in
  der Abnahmematrix.
- Erster Aufschlag enthaelt VStack, HStack, Text, Button und eine virtuelle
  Bildergalerie mit 10.000 bis 100.000 Eintraegen.
- Border, abgerundete Formen und Shadow gehoeren zum ersten Aufschlag.
  Ein experimentelles glassartiges Material zeigt die Effektarchitektur.
- Image-Loading, Metadaten-Probing, Thumbnail-Erzeugung und Caching werden vom
  Framework gesteuert. Standardquellen: lokales Dateisystem und HTTP.
- Eingabe umfasst Maus, Tastatur und einfache Touch-Gesten (Tap, Drag-Scroll,
  kinetisches Scrollen). Siehe Abschnitt 6.
- Beispiele liegen unter cmd/example-*; Tests auf der Zielhardware uebernimmt
  der Nutzer. Lokal werden Korrektheit, Builds und Allokationsziele geprueft.

60 fps ist ein zu validierendes Ziel, keine bereits nachgewiesene Eigenschaft.
Insbesondere Glass-Effekte und kaltes Nachladen erhalten eigene Messszenarien.
Die Abnahmeschwellen stehen in Abschnitt 13.

## 2. Was wir von Nago uebernehmen

Uebernehmen: normale Go-Funktionen zur Komposition, kleine Value-Views,
typisierten State und Widgets als Kombination weniger Primitiven.

Nicht uebernehmen: die Gleichsetzung von Renderknoten und Transportprotokoll,
vollstaendige Root-Rebuilds fuer jede visuelle Aenderung, window-weite State-IDs,
Callstack-basierte Identitaet und reflektive Gleichheitspruefung als Standard.

Nagos Browser erledigt Layout, Text, Scrolling und viele Effekte selbst. Gift
muss diese Aufgaben ausdruecklich modellieren; der Desktop-Renderer ist nicht
bloss ein alternativer Ausgabekanal fuer denselben Protokollbaum.

Es wird kein Nago-Code kopiert. Dessen eigene Lizenz ist nicht mit einer
allgemeinen Erlaubnis zur Uebernahme gleichzusetzen.

## 3. Package-Struktur

```text
gift/                       Modulwurzel, Package gift
  geom/                     Geometrie und numerische Einheiten
  ui/                       Views, Modifier und zusammengesetzte Controls
  asset/                    Bildquellen, Collections, Lade- und Cache-Service
  render/                   Backend-Vertrag, Display-Listen, Ressourcen
  backend/
    ebiten/                 Fenster, Eingabe, Text und GPU-Ausgabe
  metrics/                  Messung und Bericht, hinter Build-Tag giftmetrics
  gifttest/                 Testharness fuer Anwendungen, siehe Abschnitt 13
  font/
    inter/                  Eingebettete Schrift, optional, siehe Abschnitt 17
    ibmplexmono/            Eingebettete Schrift, optional, siehe Abschnitt 17
  icon/
    outline/                Eingebettete Icons, optional, siehe Abschnitt 21
    solid/                  Eingebettete Icons, optional, siehe Abschnitt 21
  clipboard/                Zwischenablage per purego, optional, Abschnitt 19
  internal/
    scene/                  Indexierte retained Nodes und Handles
    layout/                 Layoutalgorithmen, Masonry-/Zeilenindex
    text/                   Shaping, Messung, Zeilenumbruch, Glyphenschluessel
    stress/                 Parametrisierte Lastszene als Fixture
    example/                Gemeinsame Hilfen der Beispiele
    icon/                   Icon-Format, Rasterer und Stroker
    iconsvg/                SVG-Teilmenge, nur fuer den Generator
  cmd/
    example-counter/
    example-gallery/
    example-effects/
    gift-stress/
    gift-icongen/
```

Stand nach Review-Gate 8. `metrics`, `gifttest` und `internal/stress` sind
nach dem urspruenglichen Entwurf dazugekommen und standen bisher nicht im
Baum. `cmd/example-layout` ist entfallen; siehe Abschnitt 12 Schritt 2.

`font/*`, `icon/*` und `clipboard` sind mit den Abschnitten 17, 21 und 19
dazugekommen. Sie enthalten Bytes und eine Registrierung, sonst nichts:
keine Views, kein Layout, keine Ein-/Ausgabe. Sie importieren `ui`, weil der
Typ, den sie liefern oder registrieren, dort liegt; sie stehen damit
**oberhalb** von `ui`, in derselben Stellung wie `cmd/`. Kein Paket innerhalb
von gift nennt sie, sie sind ausschliesslich ueber einen Side-Effect-Import
oder eine ausdrueckliche Nennung durch die Anwendung erreichbar. Die Richtung
der Abhaengigkeiten aus diesem Abschnitt bleibt dadurch unveraendert.

`internal/iconsvg` ist bewusst intern und wird nur vom Generator benutzt: es
ist eine Teilmenge von SVG fuer genau einen Korpus und keine Zusage, SVG zu
koennen.

Die Modulwurzel wird als `github.com/worldiety/gift` importiert. Kein
weiteres Verzeichnis gift innerhalb des Moduls und kein generischer
pkg/-Sammelordner.

| Package | Verantwortlichkeit | Darf nicht enthalten |
| --- | --- | --- |
| gift | App, View-Vertrag, Build-Context, Scope, State, Binding, Lifecycle, Scheduling | Ebitengine-Typen, Datei-/HTTP-Loader |
| geom | Point, Size, Rect, Insets, Constraints, Transformationen | Views, State, Backend |
| ui | Layout-Views, Text, Image, Button, Gallery, Styles und Material-Deklaration | GPU-Objekte, Netzwerkarbeit |
| asset | Source, Metadata, Collection, asynchrone Pipeline, persistenter Thumbnail-Cache | Views, UI-State, Ebitengine |
| render | Backend-Vertrag, Zeichenoperationen, Textmessung, Ressourcen-Handles, Faehigkeiten | Widgets, Transportprotokoll, App-State |
| backend/ebiten | Ebitengine-Adapter, Glyphenausgabe, Texture-, Target- und Effektverwaltung | Galerie-Layout, fachliche Controls, Shaping-Logik |
| internal/scene | Speicherlayout und Wiederverwendung der Nodes | Abhaengigkeit zur Modulwurzel gift |
| internal/layout | Gemeinsames Layout und raeumliche Indizes | Ebitengine und asset-I/O |
| internal/text | Rendererunabhaengiges Shaping und Zeilenumbruch | GPU-Objekte, Atlasverwaltung |

Vorgesehene Abhaengigkeitsrichtung:

```text
cmd/example-* ---> ui ---> gift ---> render ---> geom
       |           |  \      |
       |           |   \     +----> internal/scene
       |           |    +--------> internal/layout, internal/text
       |           +----> asset
       +----> backend/ebiten ---> gift, render, geom, internal/text
```

`ui` benutzt `internal/layout` und `internal/text` direkt; Layoutalgorithmen
und Zeilenumbruch gehoeren nicht in die Modulwurzel. Die internen Hilfspakete
duerfen render/geom verwenden, aber nicht gift zurueckimportieren. Lifecycle und
Orchestrierung bleiben deshalb zunaechst in gift. Der asset-Service liefert
CPU-Ergebnisse; die UI-Integration reicht diese ueber den rendererneutralen
Ressourcenvertrag weiter.

`internal/text` liefert Shaping-Ergebnisse als Glyph-ID-, Position- und
Font-Referenzlisten. Nur `backend/ebiten` kennt den Glyph-Atlas. Dadurch
benutzen Messung und Ausgabe zwingend dieselben Shaping-Ergebnisse.

Skalierung erfolgt entlang dieser Grenzen, nicht durch ein Package je Widget
oder durch einen presentation/core-Sammelbereich. Loader und Caches koennen
spaeter als Unterpakete von asset ausgegliedert werden, sobald ihre APIs stabil
und eigenstaendig nuetzlich sind. Kein vorsorgliches Aufteilen in mehrere Module.

## 4. Oeffentliche API: Variante A

```go
func Counter(ctx *gift.Context) gift.View {
    count := ctx.State("count", 0)

    return ui.VStack(
        ui.Text("Counter").FontSize(24),
        ui.Text(strconv.Itoa(ctx.Read(count))),
        ui.HStack(
            ui.Button(ui.Text("-"), func() {
                count.Set(count.Get() - 1)
            }),
            ui.Button(ui.Text("+"), func() {
                count.Set(count.Get() + 1)
            }),
        ).Gap(8),
    ).Gap(16).Padding(24)
}
```

- Views sind kurzlebige Beschreibungen. Dauerhafte Identitaet liegt im Scope
  und im retained Baum, nicht in einer einzelnen View-Struct-Kopie.
- Heterogene Kinderlisten verwenden View-Interfaces nur an der Build-Grenze.
  Im Frame-Hotpath wird kein rekursiver Interface-View-Baum ausgewertet.
- Modifier geben konkrete Typen zurueck. Keine gemeinsamen Modifier-Interfaces,
  deren Methoden manche Widgets stillschweigend ignorieren.
- Generische Methoden stehen auf konkreten Typen wie Context. Der View- und
  Backend-Vertrag hat gewoehnliche, nicht generische Interface-Methoden.
- Go-Generics garantieren weder vollstaendige Monomorphisierung noch Inlining
  oder Allokationsfreiheit. Escape-Analyse und Benchmarks entscheiden.

### Styling endet an der View-Grenze

Nachtrag aus WU-C. Variante A hat eine Konsequenz, die der urspruengliche Plan
nicht ausgesprochen hat: **Modifier ueberleben die `gift.View`-Grenze nicht.**
Sie sind auf konkreten Typen deklariert; sobald ein Wert in `gift.View` geboxt
ist, sind sie weg.

Daraus folgt konkret:

- `gift.Component("row", Row).Padding(8)` kompiliert nicht und kann es nicht.
  Der sanktionierte Weg ist `ui.VStack(gift.Component(...)).Padding(8)`, also
  genau der zusaetzliche Knoten, den das Feld-Design bei Blaettern vermeidet.
- Eine Hilfsfunktion `func Card(title string) gift.View` ist vom Aufrufer nicht
  stylebar. Wiederverwendbare Widgets muessen entweder einen konkreten Typ
  zurueckgeben oder Style-Parameter entgegennehmen.
- Die API-Flaeche waechst multiplikativ: jede Modifier-Methode mal jedes Widget.
  Bei Text, Image, Button und Gallery kommen dutzende Einzeiler-Forwarder dazu,
  und der Compiler meldet nicht, wenn einer vergessen wurde.

  **Entschieden nach WU-G: nicht generieren, sondern pruefen.** Mit `Box`,
  `Stack`, `Overlay`, `Spacer` und `Text` liegen rund 70 solcher Einzeiler vor,
  die in §4 genannte Schwelle ist also erreicht. Ein Generator loest aber das
  falsche Problem: der Fehlerfall "Methode vergessen" ist ein Compilerfehler an
  der Aufrufstelle, kein stilles Fehlverhalten. Ein Generator kostet dafuer
  einen Buildschritt, einen `go:generate`-Vertrag und eine zweite Stelle, an
  der man `func (t TextView) Padding` suchen muss.

  Stattdessen: ein Test, der ueber alle exportierten View-Typen reflektiert und
  den gemeinsamen Modifier-Satz mit korrekter Signatur einfordert. Er faengt
  genau den realen Fehler, kostet rund dreissig Zeilen und haelt den Code
  greppbar. Faellig mit WU-H.

Das ist kein Argument fuer gemeinsame Modifier-Interfaces; die bleiben
abgelehnt. Es ist die ehrliche Formulierung des Preises: **Styling ist eine
Eigenschaft der konkreten Konstruktionsstelle, nicht etwas, das man auf eine
beliebige `View` anwenden kann.**

### Ownership der Kinderslices

`ui.VStack(a, b, c)` erzeugt ein frisches, vom Compiler alloziertes Slice; Gift
darf es behalten. `ui.VStack(items...)` uebergibt dagegen das Slice des Aufrufers
ohne Kopie.

Vertrag: **ab Uebergabe gehoert das Slice Gift.** Der Aufrufer darf es weder
veraendern noch weiterverwenden. Wer ein wiederverwendetes Puffer-Slice besitzt,
muss selbst kopieren oder den Builder mit expliziter Kopiersemantik nutzen.
Value-Semantik ist keine Deep-Copy-Zusage. Im Debug-Build wird die Slice-Identitaet
zwischen zwei Builds geprueft und eine Verletzung gemeldet.

### Komponenten

Eine normale Hilfsfunktion erzeugt keinen eigenen State-Scope. Wiederverwendbare
zustandsbehaftete Komponenten werden explizit als Komponenten gemountet:

```go
ui.VStack(
    gift.Component("left-counter", Counter),
    gift.Component("right-counter", Counter),
)
```

## 5. State, Binding und Lifecycle

- `ctx.State("name", initial)` liefert einmalig initialisierten, typisierten
  State innerhalb der aktuellen Komponenteninstanz. Gleiche lokale Keys in
  unterschiedlichen Komponenten sind erlaubt.
- Der Standardweg verwendet vergleichbare Werte und `==`. Fuer andere Werte
  gibt es einen ausdruecklichen Comparator-/Versionsweg, kein DeepEqual-Default.
- `ctx.Read(state)` liest und registriert die Abhaengigkeit zum aktuellen Scope.
  `state.Get()` liest ohne Abhaengigkeit, etwa in einem Eventhandler.
- `state.Set(value)` ist der eindeutige Aenderungsweg. Aenderungen werden bis zum
  naechsten Build zusammengefasst. Kein separates Notify zur normalen Verwendung.
- `state.Binding()` verbindet Lesen und Schreiben fuer Controls, ohne Ownership
  zu uebertragen. Binding-Konsumenten registrieren ihre Abhaengigkeit ebenfalls.
- State lebt bis zum Unmount seines Scopes, nicht bis zum Auslassen eines Lookups.
  Ein gecachter, nicht neu gebauter Teilbaum bleibt gemountet.
- Dynamische Kinder brauchen stabile Modell-Keys. Typwechsel unter gleichem
  State-Key wird mit einer klaren Diagnose abgewiesen.
- Abhaengigkeiten werden nach einem erfolgreichen Build aktualisiert; nicht
  mehr gelesene States invalidieren den Scope danach nicht weiter.
- UI-State gehoert einem UI-Executor. Worker-Ergebnisse werden gepostet und vor
  Annahme auf Scope-/Request-Generation geprueft. Keine Callbacks unter Locks.
- Scrolloffset, Hover, Pressed und Animation sind lokaler Praesentationszustand.
  Sie sollen keinen fachlichen Root-Rebuild erzwingen.
- Dauerhafte Galerieauswahl lebt beim Collection-Owner, nicht im recycelten Tile.

Praesentationszustand, der einen Neuaufbau ueberleben muss, liegt in der
Knoten-Nutzlast und nicht im View, im Layouter oder im Painter: diese drei
werden bei jedem Aufbau neu erzeugt. Fuer das Scrollen ist das
`ScrollIndicatorState`, fuer Steuerelemente `ControlState` — Griffpunkt einer
laufenden Geste, Ausgangswert und Startzeitpunkt einer laufenden Animation.
Geschrieben wird er aus dem Interactor und aus dem Layouter; der Layoutdurchlauf
ist die einzige Stelle, an der ein Knoten bemerken kann, dass ein *Aufbau*
seinen Wert veraendert hat. Der Painter liest ihn und schreibt ihn nie.

Zwei Regeln dazu, beide aus Review-Gate 12 und beide teuer erkauft. Erstens:
ein Nutzlastfeld, das nicht bei jedem Aufbau aus dem Element aufgefrischt wird,
wird beim **Unmount** in `nodeData.release()` geleert, neben `scroll`, `ia` und
`xform` — nicht beim Mount. Der Unterschied ist sichtbar, weil die Nutzlast
recycelt wird: ein Test, der einen Knoten in *einem* Update austauscht, trifft
den recycelten Platz nie, weil der Abgleich erst alle neuen Kinder mountet und
danach die uebrig gebliebenen abraeumt. Ein solcher Test ist gruen, auch wenn
das Leeren ganz fehlt. Zweitens: gift stellt einem deaktivierten Knoten keine
Ereignisse zu. Ein Steuerelement, das waehrend einer Geste deaktiviert wird,
bekommt sein Loslassen also nie und kann sich nicht selbst aufraeumen. Die
Geste wird dort beendet, wo die Deaktivierung angewandt wird. Ein Wachposten
im Widget selbst ist dort unerreichbar und darum kein zusaetzlicher Schutz,
sondern eine irrefuehrende Behauptung.

Ein Knoten kennt einen dritten Zustand neben gemountet und abgeraeumt:
**gemountet und aus dem Bild genommen** (`Element.Hidden`). Er wird nicht
gezeichnet, ist kein Zeigerziel und steht nicht in der Fokusreihenfolge; sein
Zustand, seine Scrolloffsets und sein halb getippter Text bleiben. Aufbau und
Layout laufen weiter, weil ein sauberer Teilbaum ohnehin nicht gemessen wird
und das Ueberspringen eines schmutzigen die Arbeit nur in den Frame
verschieben wuerde, in dem er wieder sichtbar wird — also genau dorthin, wo
sie als Ruckler auffaellt. Das ist die eine Kosten, die ein verborgener Tab
nicht spart, und sie gehoert benannt statt weggeredet.

Wer den Fokus oder einen erfassten Zeiger haelt, waehrend seine Ebene
verborgen wird, verliert ihn dort, wo verborgen wird. Ein Unmount findet nicht
statt, es raeumt also sonst niemand auf. Das ist keine Feinheit: ein erfasster
Zeiger wird ohne Treffertest zugestellt, ein Regler unter dem Finger schreibt
also weiter, waehrend ihn niemand sieht. Und jede Repaint-Anmeldung eines
verborgenen Teilbaums wird **beendet**, nicht auslaufen gelassen.

Daraus folgt eine Regel ueber diesen Abschnitt hinaus. Wird einem Knoten vom
Kern etwas weggenommen, das er selbst haelt — Fokus, Erfassung, Hover —, dann
muss er davon erfahren, auch wenn das mitten in einem Build geschieht und auch
wenn er deaktiviert ist. Sonst gibt er seine Anmeldungen nie frei. Eine
Zustellung, die waehrend der Rekonziliation nicht laufen darf, wird
**aufgeschoben** und im selben Update nachgeholt, nicht verworfen.

Eine Abhaengigkeit gehoert dem Scope, der sie gelesen hat. `ctx.Read`
registriert sie auf dem Component-Scope, in dem es aufgerufen wurde; `Get`
registriert nichts und ist das, was ein Eventhandler benutzt. Eine Wurzel, die
alle Zustands-Slots liest, ist eine Wurzel, die von allem invalidiert wird —
und ein Wurzel-Rebuild baut auch jeden verborgenen Tab neu auf, denn ein
verborgener Teilbaum wird nach der Regel oben absichtlich gebaut und
layoutet. Jeder Schirm bekommt deshalb eine eigene `gift.Component` und liest
dort nur die Slots, die er zeigt.

Beide Fehlerrichtungen sind lautlos, und das ist der Grund, warum das hier
steht: zu wenige `Read` und ein Schreibvorgang invalidiert nichts — der Wert
stimmt, der Handler laeuft, der Schirm bewegt sich nicht. Zu viele, und ein
Schreibvorgang baut Schirme neu auf, die von ihm nicht abhaengen; dann sieht
alles richtig aus und das Geraet ist langsam. Gemessen an
`cmd/example-kitchensink`, ein Tipp auf eine Zeile der Einstellungen: vorher
702 µs und 6255 Allokationen — ein voller Wurzel-Rebuild —, nachher 70 µs und
485. Auf einem Pi 4 sind das 42 Prozent gegen 4 Prozent des Frame-Budgets, pro
Tipp, und bei einem Regler pro Frame der Geste.

Noch kein allgemeiner Signal-/Effect-Graph, keine implizite Goroutine-Sicherheit
fuer State und keine magische Erkennung von In-place-Mutationen an Maps/Slices.

## 6. Frame-Modell

Dieser Abschnitt haelt fest, was Ebitengine tatsaechlich anbietet, weil das die
Invalidierungsarchitektur bestimmt.

### Ebitengine-Realitaet

- `Update` und `Draw` sind entkoppelt. Vor einem `Draw` koennen mehrere `Update`
  laufen, bei hoher Last auch keiner.
- `Draw` erhaelt den Screen. Standardmaessig wird er jeden Frame geloescht.
  Es gibt **kein** partielles Neuzeichnen des Bildschirms durch Dirty Rects.
- Ebitengine exponiert keine GL-Version, keine Extensions und keine
  GPU-Speicherabfrage. Eine capability-basierte Qualitaetswahl ist nicht moeglich.

### Konsequenz: was Invalidierung spart und was nicht

Gift zeichnet **jeden Frame die vollstaendige sichtbare Display-Liste neu.**
Es gibt keine Composite-Invalidierung im Sinne von "nur der geaenderte
Bildschirmausschnitt wird angefasst". Frueher formulierte Erwartungen dazu
werden hiermit zurueckgenommen.

Invalidierung spart deshalb gezielt CPU-Arbeit, nicht Fuellrate:

| Stufe | Was entfaellt bei Nichtaenderung |
| --- | --- |
| Build | Kein Aufruf der View-Funktionen, keine Interface-Allokationen |
| Layout | Kein Measure/Arrange, keine Neuberechnung des Galerieindex |
| Text | Kein Shaping, kein Zeilenumbruch, keine Glyph-Atlas-Aenderung |
| Paint | Keine Neuerzeugung der Display-Liste, nur Transform-Patch |
| Effekt-Cache | Kein Blur-Rerender fuer Shadow und Glass |

Reines Scrollen darf deshalb weder Text neu formen noch den View-Baum neu bauen.
Es fuehrt aber weiterhin zu einem vollen Zeichendurchlauf der sichtbaren Kacheln.
Das ist die Basislast, gegen die alle Budgets in Abschnitt 13 gerechnet werden.

### Ablauf pro Frame

```text
Update:
  Eingabe einlesen, Gesten aufloesen, Animationen taktieren
  gepostete Worker-Ergebnisse annehmen (Generation pruefen)
  falls Build-Invalidierung: betroffene Scopes neu bauen, reconcilen
  falls Layout-Invalidierung: Measure und Arrange
  Upload-Budget planen

Draw:
  Display-Liste der sichtbaren Nodes erzeugen bzw. wiederverwenden
  Material-Passes (siehe Abschnitt 8)
  Zeichenbefehle absetzen
```

Build und Layout laufen ausschliesslich in `Update`, nie in `Draw`. Uploads
werden pro tatsaechlich gezeichnetem Frame budgetiert, nicht pro `Update`.

### Idle-Verhalten

Ohne Aenderung und ohne laufende Animation zeichnet Gift eine statische Szene
weiterhin 60-mal pro Sekunde, weil Ebitengine das so vorgibt. Fuer Pi-Betrieb
ist das relevant: Dauerlast erzeugt Waerme und damit Throttling. Gegenmassnahme
im ersten Aufschlag ist `ebiten.SetTPS` samt reduzierter Zielrate im Idle als
optionale App-Politik, nicht ein eigener Praesentationspfad. Echtes
On-Demand-Rendering ist **kein** Bestandteil dieses Plans.

Praezisierung aus WU-D, in laufendem Code geprueft: `SetTPS` regelt
ausschliesslich die `Update`-Rate. Die `Draw`-Rate gibt das Display vor. Ein Pi
an 60 Hz bekommt mit `IdleTPS=10` weiterhin 60 `Draw`-Aufrufe je Sekunde. Die
Idle-Politik spart also Build, Layout und Eingabeverarbeitung, **nicht** die
Fuellrate. Das ist dieselbe Trennung wie bei der Invalidierung oben.

Ebitengine hat neben `Update` und `Draw` einen dritten Callback `Layout`. Er
ist die einzige Stelle, an der die Viewportgroesse vor dem ersten `Update`
bekannt ist. Das Backend bezieht sie von dort.

## 7. Layout, Text und Eingabe

```text
Deklaration bei relevanter Aenderung
  -> keyed Reconciliation / retained Nodes
  -> Measure und Arrange bei Layout-Invalidierung
  -> Display-Liste, Clip- und Transformdaten
  -> Backend-Ressourcen und Ausgabe
```

### Overflow-Modell

Nachtrag aus Review-Gate 2. Der urspruengliche Plan hat die Frage "der Inhalt
passt nicht" nicht beantwortet. WU-C hat sie stillschweigend beantwortet, und
zwar falsch: ein Stack hat spaeteren Kindern den verbleibenden Platz als Maximum
gegeben, diese sind auf Hoehe 0 kollabiert, ihre eigenen Kinder wurden aber
weiterhin an ihren echten Positionen gezeichnet und lagen uebereinander. Bei
40 Zeilen in einem begrenzten Viewport kollabierten 30 und stapelten sich in
einem 120-Pixel-Band. Das ist die eine Kombination, die weder ehrlich noch
sicher ist.

Verbindlich gilt ab sofort:

1. **Ein Stack misst unflexible Kinder mit unbegrenzter Hauptachse.** Er
   verteilt keinen Restplatz an sie und verknappt sie nicht der Reihe nach. Die
   Querachse bleibt begrenzt. Damit haengt die Groesse eines Kindes nicht davon
   ab, wie viele Geschwister vor ihm stehen.
2. **Flexible Kinder bekommen den Rest**, tight auf der Hauptachse, anteilig
   nach `Flex`. Ist kein Rest da, bekommen sie 0.
3. **Overflow ist erlaubt und sichtbar.** Ueberschreitet die Summe aus Kindern
   und Gaps den verfuegbaren Platz, behalten die Kinder ihre ehrlichen Groessen
   und Positionen. Der Stack meldet nach oben die von den Constraints erlaubte
   Groesse, aber der Ueberstand wird als Zahl gefuehrt und ist in `Diagnostics`
   sichtbar. Unter `giftdebug` gibt es zusaetzlich eine Diagnose mit Knoten und
   Ueberstand.
4. **Es wird nicht automatisch geclippt.** Wer Overflow abschneiden will, setzt
   `Clip(true)`. Stillschweigendes Clippen wuerde denselben Fehler verstecken,
   den Punkt 3 sichtbar machen soll.
5. Das deckt sich mit dem bereits dokumentierten Vertrag von `gift.Layouter`,
   dass eine constraint-verletzende Groesse unveraendert durchgereicht und nicht
   stillschweigend geklemmt wird. Die bisherige Implementierung hat genau dagegen
   verstossen.

Folgeregel fuer `ui.Box`: ein `Box` ist auf jeder **begrenzten** Achse gierig.
Weil ein Stack die Hauptachse nach Regel 1 unbegrenzt misst, kollabiert ein
`Box` ohne `Frame` oder `Flex` in einem Stack auf der Hauptachse und fuellt die
Querachse. In einem `ZStack` sind beide Achsen begrenzt, also fuellt er beide.
Das ist genau das Verhalten, das die Godocs beider Stellen ohnehin behaupten.

Erste Layoutregeln: numerische logische Pixel, Min-/Ideal-/Max-Groessen,
Padding, Gap, Alignment und Spacer. Text wird mit begrenzter Breite gemessen;
Zeilenumbruch und Baselines sind Bestandteil des gemeinsamen Vertrags.
Keine CSS-Strings und keine unterschiedlichen Stack-Algorithmen je Backend.

Nodes und Zeichenoperationen liegen in wiederverwendbaren Slices mit
indexbasierten Handles und Generationen. Die Lebensdauer geliehener Display-
Listen ist definiert: der Backend-Consumer darf sie nicht nach Wiederverwendung
durch den Producer lesen. Backend-eigene Kopien und GPU-Kommandos sind gesondert
zu budgetieren.

Koordinatenraum von Clip und Transform, festgelegt in WU-D: **Clips sind
Device-Space, Transformationen bilden Bounds nach Device-Space ab.**
`List.PushClip` schneidet entlang eines einzigen Stapels, und das ergibt nur in
einem gemeinsamen Raum Sinn. Sollte die Galerie spaeter in gescrolltem Raum
clippen muessen, ist das eine Verhaltensaenderung und vor Schritt 3 zu
entscheiden, kein Implementierungsdetail.

### Textstack

Shaping erfolgt mit `github.com/go-text/typesetting`, derselben Basis, die auch
Ebitengines `text/v2` verwendet. `internal/text` kapselt sie vollstaendig.

Im ersten Aufschlag enthalten:

- Horizontales LTR-Shaping, Kerning, Ligaturen nach Font-Vorgabe.
- Zeilenumbruch an Wortgrenzen nach UAX 14 in der von typesetting gelieferten Form.
- Shaping-Cache mit Schluessel aus Text, Font, Groesse und Breitenbeschraenkung.
- Glyph-Atlas im Backend mit Eviction nach Bytes und Verwendungsalter.
- Ganzzahlige Baseline-Positionierung; kein Subpixel-Positioning.

Bewusst nicht enthalten: Bidi und RTL, vertikale Schrift, automatische
Fallback-Fontketten ueber mehrere Schriften, Hinting-Varianten, Texteditor, IME.
Die Fallback-Kette ist die wahrscheinlichste erste Erweiterung nach dem MVP;
die API von `internal/text` wird dafuer offen gehalten, aber nicht vorgebaut.

Shaping ist erfahrungsgemaess der groesste Einzelposten in Schritt 2. Er wird
deshalb dort getrennt ausgewiesen.

### Eingabe

Eingabe, Clip und Hit-Testing verwenden dieselben Koordinatentransformationen.

- Maus: Hover, Pressed, Pointer-Capture, Wheel-Scrolling.
- Tastatur: Fokusreihenfolge, Disabled, Aktivierung per Space/Enter,
  Pfeilnavigation in der Galerie.
- Touch: Tap, Long-Press, Drag-Scroll und kinetisches Scrollen mit Reibung.
  Touch nachtraeglich einzubauen waere teuer, weil es die Pointer-Capture- und
  Scroll-Semantik beruehrt.

  **Korrigiert nach WU-H.** Die urspruengliche Begruendung lautete zusaetzlich,
  die Zielhardware sei typischerweise ein Pi-Display. Das ist fuer den
  Desktop-Backend falsch und in der Quelle geprueft: Ebitengine dokumentiert an
  `AppendTouchIDs` selbst "AppendTouchIDs always does nothing on desktops".
  Unter Raspberry Pi OS mit X11/XWayland - der in Abschnitt 1 festgelegten
  Plattform - kommt ein Touchscreen als Maus an. Der Touch-Pfad ist real und
  getestet, wird aber von Android- und iOS-Builds ausgeuebt, nicht vom Pi. Das
  Architekturargument traegt, das Hardwareargument nicht.

  Ebitengine hat ausserdem **keine Tastenwiederholung**. Die Suche im
  gepinnten Modul findet `Repeat` nur im Gamepad-Code. Das Naechstliegende ist
  `inpututil.KeyPressDuration`, das Ticks zaehlt; Anfangsverzoegerung und Rate
  muessten darauf aufgebaut werden. Fuer den ersten Aufschlag ist das ohne
  Belang, weil Space/Enter und Tab auf der Flanke ausloesen. Ein Texteditor
  braeuchte es, und der ist in Abschnitt 14 ausgeschlossen.

Ein gemeinsames Pointer-Modell abstrahiert Maus und Touch; Touch-Ereignisse
erzeugen keine synthetischen Hover-Zustaende. Multitouch beschraenkt sich auf
die Erkennung und Verwerfung zusaetzlicher Finger. Pinch-Zoom, Rotation und
vollstaendige native Accessibility-Bridges sind nicht Teil des MVP.

**Nachtrag Abschnitt 26.** Multitouch ist jetzt opt-in pro Knoten
(`Element.MultiTouch`, wie `isMultipleTouchEnabled` bei UIKit): weitere Finger
erreichen nur einen Knoten, der sie verlangt, alle anderen werden weiter
verworfen und gezaehlt. Die Begruendung "unter X11 kommt Touch als Maus an"
gilt nicht mehr, weil das Backend den Touchscreen unter Linux direkt liest.

## 8. Border, Shadow und Glass

Effekte sind rendererneutrale Deklarationen und keine Ebitengine-Shader im
Anwendungscode. Die Schreibweise soll beispielsweise so aussehen:

```go
ui.VStack(
    ui.Text("Library").FontSize(24),
    ui.Button(ui.Text("Import"), importPhotos),
).
    Gap(12).
    Padding(20).
    Background(ui.Glass().Quality(ui.Adaptive)).
    Border(ui.Border{Width: 1, Color: ui.RGBA(255, 255, 255, 90)}).
    CornerRadius(18).
    Shadow(ui.Shadow{Blur: 16, OffsetY: 4, Color: ui.RGBA(0, 0, 0, 70)})
```

### Semantik

- Zunaechst benannte Style-Felder mit definierter Zeichenreihenfolge:
  Shadow, Hintergrund/Material, Inhalt, Border. Wiederholtes Setzen ersetzt den
  jeweiligen Style-Wert. Keine Behauptung identischer SwiftUI-Modifier-Semantik.
- Border liegt innerhalb der Bounds und aendert das Layout nicht.
- CornerRadius bestimmt die Hintergrund-/Borderform; Content-Clipping wird
  explizit eingeschaltet. Ein Glass-Backdrop wird an seiner Materialform geclippt.
  Ein Bild traegt seine Rundung selbst: `render.OpImage` rundet seine Bounds
  mit derselben Kantenglaettung wie ein gerundetes Rechteck, und ein
  Cover-Zuschnitt ist `render.ImageCover` statt eines Clips. Ein formgenauer
  Clip fuer beliebige Kinder bleibt offen; der Clip-Stapel kennt nur Rechtecke.
- Shadow erweitert die Paint-Bounds, aber nicht Layout oder Hit-Area.
  Eltern-Clips gelten auch fuer den Schatten.
- Farben und Alpha-Konventionen werden an der Backend-Grenze klar festgelegt;
  Ebitengine erwartet premultipliziertes RGBA fuer Pixel-Uploads.

### Warum Glass mehrere Passes braucht

Die naheliegende Frage lautet, warum ein einziger Shader nicht genuegt. Zwei
unabhaengige Gruende:

1. **Kein Framebuffer-Read.** Ein Fragment-Shader kann das Renderziel, in das er
   schreibt, nicht gleichzeitig lesen. GL ES und Kage bieten weder programmierbares
   Blending noch `framebuffer_fetch`. Der Hintergrund muss als samplebare Textur
   vorliegen. Das erzwingt mindestens ein separates Render-Target und ist der
   eigentliche Grund fuer den Mehraufwand, unabhaengig vom Blur.
2. **Blur ist eine weite Faltung.** Ein Gauss mit Radius 16 braucht 33 Taps,
   separiert 2x33. Bei 1080p ist das auf VideoCore VI nicht bezahlbar.

Grund 2 laesst sich weitgehend aufloesen: **Downsampling ist der Blur.** Bei
1/8 Aufloesung wirkt ein 4-Tap-Kernel wie Radius 24 im Vollbild. Mit Dual-Kawase
bleibt die Kette auf die **Materialregion** begrenzt statt auf den Bildschirm.
Fuer ein 1920x200-Panel sind das rund 0,6 MPixel statt 66 Taps auf 0,38 MPixel.

Zwei Korrekturen aus WU-S:

- **"Unter 1,5x" ist arithmetisch unerreichbar** und widersprach dem eigenen
  Rechenbeispiel oben. Die Down-Kette schreibt 1/4 + 1/16 + 1/64, die Up-Kette
  dasselbe rueckwaerts **plus die volle Region**. Bei drei Stufen, was dem
  Standardradius entspricht, sind das 0,33 + 1,31 = **1,64x der Region**. Die
  genannten 0,6 MPixel auf 0,384 MPixel Region sind selbst 1,56x. Verbindlich
  ist die Arithmetik, nicht der Satz. Mit Kopie und Composite kostet das ganze
  Material rund 3,6x die Region.
- **Ebitengine filtert fuer Kage-Shader immer nearest.** `DrawTrianglesShaderOptions`
  hat kein Filter-Feld, und `internal/graphicsdriver/opengl/context.go:232`
  setzt `GL_NEAREST` fest; lineare Filterung wird nur in den eingebauten Shader
  hineingeneriert. "Downsampling ist der Blur" stuetzt sich aber auf die
  bilineare Mittelung, die ein eigener Shader damit nicht bekommt. Ein
  lehrbuchmaessiger 5-Tap-Down-Pass verwirft gegen einen Nearest-Sampler die
  Haelfte der Quelle und kriecht, sobald der Inhalt scrollt. Down- und Up-Pass
  muessen die Mittelung selbst bilden und kosten deshalb acht Fetches statt
  fuenf.

Grund 1 laesst sich nicht aufloesen, aber auf genau ein Extra-Target begrenzen:
kopiert wird nur die Materialregion.

**Korrektur aus WU-S, in der Quelle geprueft.** Der letzte Satz stimmt so nicht.
Ebitengine laesst den Screen gar nicht als Shader-Quelle zu:
`internal/atlas/image.go:732` panickt mit "atlas: a screen image cannot be
created as a source". Die Regionskopie braucht also eine Quelle, die es erst
zu erzeugen gilt. Gift rendert einen Frame, der ein Material enthaelt, in ein
bildschirmgrosses Offscreen und blittet es. Das kostet **ein
bildschirmgrosses Target, ein Clear und ein Blit je Frame**, und zwar nur
solange ein Material sichtbar ist. Abschnitt 11s "Keine Vollbild-Textur pro
Widget" gilt weiterhin - es ist eine je Fenster, nicht je Widget -, aber das
Kostenmodell dieses Abschnitts war zu guenstig und ist hiermit korrigiert.

### Die zwei Qualitaetsstufen

**Reduced, Standard:** kein Blur. Tint, Fresnel-artige Kantenaufhellung,
Specular-Highlight und eine leichte, normalenbasierte Brechungsverschiebung auf
dem ungeblurrten Hintergrund. Das ist der Default auf Pi 4.

**Korrektur aus WU-S: "ein einziger Shader-Pass, kein Extra-Target" war
widerspruechlich.** Die Brechung tastet den Hintergrund ab, und das setzt
voraus, dass er als Textur vorliegt - was Grund 1 oben selbst feststellt.
Reduced kostet daher eine Regionskopie und einen Composite-Pass. Ohne Brechung
waere es ein Pass, saehe aber nicht nach Glas aus.

Und zur Wirkung, ehrlicher als die Vorgaenger-Revision: Reduced liest sich als
Glas, **weil sich der Hintergrund am Rand biegt**, nicht wegen des Tints. Ueber
einem flachen Hintergrund ist es ein leicht getoentes Rechteck mit hellen
Kanten, weil nichts da ist, was sich biegen koennte. Das behebt kein Shader.
Die Formulierung "optisch ueberraschend nah an Glas" wird auf diesen Befund
zurueckgenommen.

**Full, experimentell:** Regionskopie, Dual-Kawase-Kette auf der Region,
Composite-Pass mit Tint, Brechung, Highlight und Grain. Drei Stufen, alle auf die
Materialregion begrenzt.

| Effekt | Erster Ansatz | Begrenzung |
| --- | --- | --- |
| Border / Radius | Analytische Geometrie im gemeinsamen Shape-Shader | Kein Offscreen-Bild pro Widget |
| Shadow | Wiederverwendbare, gecachte Formmaske; Blur nur bei Form-/Parameterwechsel | Cache nach Bytes, Schattenumfang und Blur-Radius begrenzen |
| Glass Reduced | Regionskopie + Composite-Pass | Kein echter Hintergrund-Blur |
| Glass Full | Regionskopie + Dual-Kawase + Composite | Experimentell, Flaechen- und Speicherbudget je Frame |

Die Display-Liste muss Materialregionen, Z-Reihenfolge und
Hintergrundabhaengigkeiten ausdruecken. Das Backend erzeugt die Zwischenziele;
kein `ReadPixels` und keine zyklischen Texturabhaengigkeiten.

Der Backdrop enthaelt nur vorher gezeichnete Inhalte, nicht das Material selbst
oder seine Kinder. Beim Scrollen darunter wird er dirty. Ein statischer
Glasrahmen bedeutet deshalb nicht, dass sein Blur wiederverwendbar ist.

### Adaptive ist messungsbasiert

Da Ebitengine keine Capabilities exponiert, kann `Adaptive` nicht
geraetebasiert entscheiden. Es ist eine **messungsbasierte Politik**: ein
gleitendes Fenster der Frametimes und des Materialflaechenbudgets entscheidet
zwischen Full und Reduced, mit Hysterese und einer Mindesthaltedauer je Stufe,
damit kein Flackern entsteht. Die effektive Stufe ist in der Diagnostik sichtbar.
Anwendungen koennen eine Stufe fixieren; fuer vergleichbare Messungen ist das
verpflichtend.

Im ersten Beispiel: ein Glass-Panel ueber scrollender Galerie. Kein Blur fuer
jede einzelne Kachel. Kein Anspruch auf eine pixelgenaue Nachbildung von Apples
Liquid Glass; das eigene Material bleibt experimentell.

## 9. Assets und automatische Bildoptimierung

```go
type Source interface {
    Metadata() Metadata
    Open(context.Context) (io.ReadCloser, error)
}

type Metadata struct {
    ID       ID
    Revision string
    Width    uint32
    Height   uint32
    MIMEType string
}
```

Metadata ist nicht blockierend; unbekannte Felder bleiben leer. Bekannte
Abmessungen beziehen sich auf die orientierte Darstellung. Open liefert einen
frischen Reader; Gift schliesst ihn. Der Reader muss nicht seekbar sein.

`asset.File(path)` und `asset.HTTP(url)` konstruieren Quellen ohne blockierende
I/O. Vor dem Probing gilt: die `ID` wird aus dem bereinigten absoluten Pfad
beziehungsweise der normalisierten URL gebildet, die `Revision` bleibt leer und
wird nach dem Probing aus mtime und Groesse beziehungsweise aus ETag oder
Last-Modified gesetzt. Eine leere Revision bedeutet "noch nicht validiert", nicht
"unveraenderlich". Die Anwendung kann eigene Asset-Objekte liefern. Vorhandene
Provider-Thumbnails koennen spaeter ueber eine optionale Erweiterung genutzt werden.

Eine app-weite Pipeline dedupliziert Requests und verwaltet:

1. Metadaten-Probing und Validierung der Quelle.
2. Priorisierte, bytebegrenzte Fetch-/Decode-Auftraege.
3. Orientierung und Thumbnail-Erzeugung in wenigen Pixelgroessen mit Hysterese.
4. Persistenten Thumbnail-Cache mit konfigurierbarem Verzeichnis und Budget.
5. Begrenzten CPU-Pixelcache und eine begrenzte Ready-Queue.
6. Upload-Admission und explizit freigegebene Backend-Ressourcen.

Sichtbare Bilder haben Vorrang vor richtungsabhaengigem Prefetch. Abgebrochene
Arbeit darf die aktuelle Ansicht nicht ueberschreiben. Content-Version,
Request-Generation und Tile-/Texture-Slot-Generation sind getrennte Konzepte.

Cache-Keys enthalten Revision, Groesse, Orientierung und Verarbeitungsversion.
Dateiquellen werden neu validiert, HTTP nutzt verfuegbare Validatoren und eine
definierte Freshness-Policy. Ohne Validator ist eine URL keine ewige Revision.
Authentifizierte Quellen brauchen getrennte Cache-Namespaces; Zugangsdaten
gehoeren nicht in Dateinamen oder Diagnoseausgaben. Disk-Caching ist abschaltbar.

Jeder Pipelineabschnitt hat ein Bytebudget. Ein Channel-Limit allein genuegt
nicht. Decode-Arbeit reserviert Speicher nach Bildabmessungen und Codec-Risiko,
nicht nach der spaeteren Thumbnailgroesse. Begrenzte komprimierte Eingabedaten,
Pixelgrenzen, Timeouts und Fehler-/Retry-Zustaende gehoeren zum ersten Prototyp.

Die Standarddecoder sind nicht beliebig abbrechbar und allokieren. CPU-Decoding
wird daher nicht als GC-frei bezeichnet. Ein Kontextabbruch verhindert weitere
Arbeit und die Veroeffentlichung veralteter Ergebnisse, kann aber bereits laufende
Codec-Berechnungen nicht generell sofort stoppen.

## 10. Galerie und grosse Collections

```go
func Gallery(ctx *gift.Context, photos *asset.Collection) gift.View {
    return ui.ImageGallery(photos).
        Layout(ui.Masonry().MinColumnWidth(240).Gap(8)).
        OnSelect(func(id asset.ID) {
            // Detailansicht oeffnen.
        })
}
```

**Korrektur aus WU-N: die obige Skizze ist so nicht baubar.** `ui.ImageGallery(photos)`
unterstellt, dass die Galerie aus einer View-Konstruktion entsteht. Ein
View-Wert wird aber bei jedem Build weggeworfen, und was ueberleben muss, ist
der Index ueber 100.000 Eintraege samt Tile-Bindungen - 2,4 MB, deren
Neuaufbau je Build O(N) waere. Richtig ist
`ui.ImageGallery(ui.NewGallery(photos))`: das Galerieobjekt gehoert der
Anwendung und ist langlebig, die View ist die kurzlebige Deklaration darauf.
Nebeneffekt: die dauerhafte Selektion liegt damit automatisch beim
Collection-Owner, wie Abschnitt 5 es verlangt.

**Tiles werden gepoolt, nicht ge- und entmountet.** Scrollen bindet Slots eines
positionsbasierten Pools im Layout um, statt Knoten zu mounten. Das ist nicht
nur billiger, es macht den in Abschnitt 13 geforderten Test
"keine falschen Bilder nach Tile-Recycling" ueberhaupt erst moeglich: eine
Umsetzung, die mountet und entmountet, recycelt nichts, und der benannte
Fehlerfall kann dort gar nicht auftreten. Preis, ehrlich benannt: Scrollen
loest ein Relayout der Galerie aus, wo ein gewoehnlicher Scroller nur neu
zeichnet. Deshalb ist das Verhalten per `ScrollSpec.Virtual` opt-in und faellt
`ui.ScrollView` nicht zur Last.

**Der Runtime-Vertrag wurde dafuer erweitert.** Virtualisierung war mit der
bestehenden `gift`-API nicht ausdrueckbar. Neu, Stand nach WU-O:
`ScrollSpec.Virtual`, `LayoutContext.RequestLayout`, `RequestBuild`,
`AnchorScroll`, `Invalidator`, `Node`, `Pass` und `gift.ScrollInteractor`.
Das ist die Naht, die jeder virtualisierende Container braucht, und keine
Galerie-Spezialitaet - aber es ist eine echte Verbreiterung des Vertrags und
gehoert deshalb hier vermerkt.

Zwei Einschraenkungen dazu, aus Review-Gate 4 und bewusst offengelassen:

- `AnchorScroll` (frueher `SetScrollOffset`) schreibt waehrend des Layouts ohne
  Invalidierung. Das ist nur solange gefahrlos, wie nichts stromabwaerts den
  Offset bereits gelesen hat. Statt sich auf Disziplin zu verlassen, panickt
  die Methode jetzt, sobald der Layouter schon ein Kind gemessen hat - die
  Bedingung ist damit erzwungen und nicht bloss dokumentiert.
- `ScrollSpec.Virtual` ist als Eigenschaft des Scrollens benannt, ist aber
  eine Eigenschaft des *Layouters*: es bedeutet nur "markiere Layout statt
  Paint". Es gibt genau einen Konsumenten und keinen zweiten plausiblen, der
  nicht ebenfalls eine virtualisierte Sammlung waere. Es bleibt vorerst, gilt
  aber nicht als abgeschlossener Teil der oeffentlichen Flaeche.

ImageGallery besitzt seinen Scrollbereich und verwendet denselben Bildservice
wie `ui.Image(source)`. Sie ist eine Komposition ueber einem internen lazy
Grid-/Viewport-Kern, kein eigener Backend-Zeichenbefehl.

Die Collection liefert geordnete, stabile IDs, Revisionen und Metadatenzugriff
ohne I/O im Frame. Strukturupdates werden versioniert publiziert. Es braucht
keinen State und keinen View pro Katalogeintrag. Die konkrete Collection-API
wird mit den ersten File-/HTTP-Beispielen festgelegt, nicht mit einer Datenbank
oder einem Dateisystem-Scanner gleichgesetzt.

- Masonry: feste Spaltenbreite, variable Hoehe, Einfuegen in die kuerzeste Spalte.
- Justified/Brick: gemeinsame Zeilenhoehe, variable Breiten und gefuellte Zeilen.
  Beide Layouts sind Teil des Galerie-Aufschlags; Masonry wird zuerst umgesetzt.
- Globale Layoutmetadaten duerfen O(N) Speicher brauchen. Sichtbare Nodes,
  Requests und Texturen bleiben durch Viewport und Budgets begrenzt.
- Pro Spalte beziehungsweise Zeile werden sichtbare Intervalle binaer gesucht.
  Der Scrollpfad durchsucht nicht alle 100.000 Eintraege.

  Praezisierung aus WU-M: die Abfrage ist **nicht** O(log N + k), und das kann
  sie bei Masonry auch nicht sein. Eine Suche je Spalte ergibt
  O(c · log(N/c) + k), wobei c eine Konstante des Viewports ist und nicht von
  N abhaengt. Gemessen bei hundertfachem N: Bruteforce 96-fach, Masonry
  4,2-fach, Justified 1,7-fach. Der Rest ueber dem reinen log-Term sind
  Cache-Misses auf einem 2,4-MB-Array. In absoluten Zahlen kostet die Abfrage
  bei 100.000 Eintraegen 1,2 µs (Masonry) beziehungsweise 0,2 µs (Justified)
  von 16,67 ms. Sollte das je stoeren, ist der Hebel ein zusammenhaengendes
  `y`-Array je Spalte, nicht ein anderer Algorithmus.
- Die Abfrage ist nur deshalb allokationsfrei, weil sie an ein Slice des
  Aufrufers anhaengt. Das ist ein Vertrag **an den Aufrufer** und muss bis in
  die Galerie durchgehalten werden.
- Initiales Layout, Sortierung und Spaltenwechsel duerfen O(N) Arbeit benoetigen;
  sie werden ausserhalb des Frame-Hotpaths oder inkrementell berechnet und
  versioniert uebernommen. Ergebnisse veralteter Layouts werden verworfen.
- Fehlende Abmessungen verwenden vorlaeufige Seitenverhaeltnisse. Korrekturen
  werden gebuendelt; stabile Bild-ID plus lokaler Offset erhalten den Scrollanker.

  Praezisierung aus WU-M: der Anker ist **geteilt**. Der lokale Offset lebt im
  Layoutindex, die stabile Bild-ID in der Collection. Der Index fuehrt bewusst
  keine IDs mit: das kostete acht Byte je Eintrag plus eine ID-nach-Position-
  Abbildung, und die Collection besitzt beides ohnehin. Beim Resize und bei
  Korrekturbuendeln genuegt der Offset allein; nur eine Umsortierung braucht
  zusaetzlich die ID.

  Ausserdem offen gelassen und jetzt entschieden: ein Korrekturbuendel
  verschiebt das bereits uebernommene Layout **nicht**. Wann der Inhalt
  nachrueckt, ist eine Ankerentscheidung und gehoert der Galerie. Sonst
  ruckelte der Viewport mitten im Frame, ohne dass vorher ein Anker genommen
  wurde.
- Sehr grosse Dokumentpositionen werden erst nach Abzug des Viewport-Ursprungs
  in float32-GPU-Koordinaten konvertiert.
- Schnelle Spruenge zeigen Platzhalter, statt auf Laden oder Decode zu warten.
- Selektion und Tastaturnavigation folgen stabilen IDs und der logischen
  Collection-Reihenfolge, nicht dem zufaelligen Speicherplatz einer Kachel.

100.000 RGBA-Thumbnails mit 256x256 Pixeln benoetigen etwa 24,4 GiB reine
Pixeldaten; 256 solcher Thumbnails etwa 64 MiB. Nur der begrenzte Arbeitsbestand
darf resident sein. Ein Beispielindex mit 32 Byte pro Eintrag benoetigt dagegen
bei 100.000 Eintraegen rund 3,1 MiB, ohne weitere Metadaten und Indizes.

## 11. Performance-Vertrag und bekannte Risiken

Das Ziel lautet: keine laufenden Heap-Allokationen im aufgewaermten Gift-Pfad fuer
unveraenderten Bildbestand und reine Scroll-/Transform-Updates.

Praezisierung, damit der Vertrag pruefbar bleibt. Er gilt fuer den Frame-Pfad
**ohne Build**: Eingabeverarbeitung, Indexsuche, Transform-Patch, Erzeugung der
Display-Liste und Absetzen der Zeichenbefehle. Ausdruecklich **nicht** erfasst,
sondern getrennt gemessen:

- Build. `ui.VStack(a, b, c)` alloziert per Definition das variadische Slice und
  boxt die Kinder in View-Interfaces. Das ist gewollt und deshalb ist Build
  invalidierungsgesteuert, nicht allokationsfrei.
- Neu sichtbare Tiles, Bild-Decoding, Thumbnail-Erzeugung.
- Backend-interne Allokationen von Ebitengine.
- **Shaping-Cache-Misses.** Praezisierung aus WU-F. Textmessung findet im
  Layout statt, und Layout ist Teil des Vertrags. Der Vertrag gilt aber nur
  fuer bereits geshapten Text: ein Cache-Hit ist mit 0 B/op gemessen, ein Miss
  alloziert rund 5 KB in harfbuzz und laesst sich ohne eigenen Shaper nicht
  vermeiden. Der Vertrag lautet also genau: **Layout ist allokationsfrei fuer
  Text, den es schon gesehen hat.**

  Das ist kein Wortspiel, sondern eine Abnahmebedingung. Ein Label, das sich
  aendert, ist ein Build und damit ohnehin ausgenommen. Eine Beschriftung, die
  sich ohne Rebuild aendert, oder eine Galerie, die staendig neue Bildtitel in
  den Viewport scrollt, verfehlt den Vertrag dagegen in jedem Frame.

  Stand nach WU-N: **dieses Messszenario hat noch keinen Gegenstand.** Die
  Galerie zeichnet bewusst keine Kacheltitel, gerade weil ein Titel je Kachel
  den Vertrag in jedem Frame brechen wuerde. Die Identitaet einer Kachel ist
  stattdessen ueber `Gallery.Bindings` abfragbar. Sobald Kacheltitel dazukommen
  - sei es in Schritt 4 oder spaeter - muss die Messung zusammen mit ihnen
  kommen und nicht danach.

  **Stand nach WU-AK: das Messszenario hat einen Gegenstand, und der Befund
  ist schaerfer als die Praezisierung oben.** `ui.List` ist die erste
  Komponente, die genug verschiedene Absaetze gleichzeitig zeichnet, um das
  Budget des prozessweiten Shapers zu ueberschreiten
  (`internal/text.DefaultMaxBytes`, ein Mebibyte, rund 1500 kurze Absaetze).
  Der Vertrag faellt dann nicht je Aenderung aus, sondern ganz: gift verwirft
  beim Zeichnen nichts, also fragt jeder sichtbare Absatz in jedem Frame den
  Shaper. Ein Absatz zu viel, und die LRU verdraengt Eintraege, die derselbe
  Frame gleich wieder braucht — die Szene verfehlt den Cache danach bei
  **allen** Absaetzen, in **jedem** Frame, auch wenn sich nichts geaendert
  hat. Gemessen: 600 Zeilen mit je zwei eigenen Beschriftungen kosten 0
  Allokationen je Ruheframe, 800 Zeilen 48 800.

  Drei Eigenschaften machen das gefaehrlicher als eine Komponentengrenze.
  Es ist eine Eigenschaft der **Szene** und nicht eines Widgets, betrifft
  also jeden kuenftigen Stapel von Beschriftungen, jede Tabelle und jede
  Protokollansicht. Es **skaliert nicht mit der CPU**, ein schnelleres Geraet
  verschiebt die Kante also nicht. Und **nichts meldet es**: das einzige
  Symptom ist ein langsames Geraet. Die Erklaerung gehoert deshalb nach
  `internal/text` und zu `ui.TextView`, wo ein Anwendungsautor sie findet,
  und nicht in die Godoc der Komponente, an der sie zufaellig auffiel.

Kein globales GC-Abschalten, keine unsafe-Arena als Ausgangspunkt.

- Ebitengine 2.10.1 baut auf Desktop ohne CGO; Linux braucht weiterhin native
  Laufzeitbibliotheken, Display-Server und einen funktionierenden Grafiktreiber.
- Der OpenGL-Uploadpfad kann glFinish vor Pixel-Updates ausfuehren. Hintergrund-
  Decoding allein garantiert daher keine ruckelfreien Uploads.
- Uploads erhalten ein Budget pro gezeichnetem Frame, nicht pro Update-Aufruf.
  Ebitengine kann mehrere Updates vor einem Draw ausfuehren.
- CPU-Zeit am Upload-Aufruf ist wegen interner Queues kein GPU-Zeitmass.
- Zunaechst automatischer Ebitengine-Atlas und explizites Deallocate bei Eviction.
  Logische Pixelbytes sind kein exaktes GPU-Budget: Padding, Fragmentierung,
  Atlaswachstum, Zwischenziele und Staging brauchen zusaetzlichen Speicher.
- Falls noetig, wird ein begrenzter Pool eigener Atlas-Seiten verglichen.
  Keine vorsorgliche eigene Atlas-Engine ohne Messung.
- Keine Vollbild-Textur pro Widget, kein GPU-Readback im Framepfad, kein globales
  Umsortieren transparenter Inhalte nur zur Verringerung von Draw Calls.
  **Korrigiert nach WU-S.** Es gibt zwei zugelassene Ausnahmen, nicht eine.
  Die Regionskopie aus Abschnitt 8 ist auf die Materialregion begrenzt. Dazu
  kommt ein **bildschirmgrosses Szenen-Target**, weil Ebitengine den Screen
  nicht als Shader-Quelle zulaesst und die Backdrop-Quelle deshalb erst
  erzeugt werden muss. Es ist eine Textur **je Fenster**, nicht je Widget, und
  wird nur bezahlt, solange ein Material sichtbar ist - nicht bloss, solange
  eines in der Display-Liste steht. Wer diesen Unterschied nicht erzwingt,
  zahlt bei 1080p acht Megabyte, ein Clear und einen Vollbild-Blit je Frame
  fuer ein Panel, das niemand sieht.
- GOMEMLIMIT begrenzt nicht GPU-/Treiber-/Gesamtprozessspeicher. Gerade beim Pi
  konkurrieren CPU und GPU um gemeinsamen physischen Speicher.
- Vollredraw jedes Frames ist die Basislast. Fuellratenprobleme lassen sich nicht
  durch bessere Invalidierung loesen, sondern nur durch weniger oder kleinere
  ueberlappende Zeichenoperationen.

## 12. Implementierung in pruefbaren Schritten

Die Groessenangaben sind grobe Kalibrierung fuer eine Person, keine Zusagen.
Schritte 1 bis 3 sind zusammen ein vollstaendiges Retained-Mode-Toolkit samt
eigenem Textstack; das ist Personenmonate, keine Sprints.

### Schritt 1: Fundament und Stacks

Ebitengine 2.10.1 pinnen, CGO-freien Desktop-Build und Linux/arm64-Cross-Build
pruefen. View-/Scope-Vertrag, State/Binding, retained Speicher, Frame-Modell aus
Abschnitt 6 und Backend-Vertrag implementieren. VStack, HStack, Spacer, Frame,
Padding und Alignment umsetzen.

Ergebnis: ein Layout-Beispiel mit stabiler Identitaet und rendererunabhaengigen
Layout-/State-Tests. Keine leeren Public-Packages nur zur Vorwegnahme des Plans.

**Go/No-Go vor Schritt 2.** Alle vier Kriterien muessen erfuellt sein:

1. `CGO_ENABLED=0` Build fuer host und linux/arm64 erfolgreich.
2. Fenster laeuft auf Pi 4 mit einer nichttrivialen statischen Stack-Szene
   stabil bei 60 fps.
3. Allokationsbenchmark des Frame-Pfads nach Warmup bei 0 B/op.
4. Layout- und State-Tests laufen ohne Fenster und ohne GPU.

Wird eines verfehlt, wird der Plan revidiert statt fortgesetzt.

### Schritt 2: Text, Button und einfache Effekte

**Zurueckgenommen: das `fwidth`-Risiko.** Die vorherige Revision hat
Screen-Space-Ableitungen im Shape-Shader als "Unbekannten mit der groessten
Varianz" gefuehrt, mit der Begruendung, Ebitengine koenne auf dem Pi auf
GLSL ES 1.00 zurueckfallen, wo `dfdx`/`dfdy` die Erweiterung
`OES_standard_derivatives` verlangen. **Das war falsch und ist in der Quelle
geprueft.** Ebitengine 2.10.1 hat gar keinen ES-1.00-Pfad:
`internal/shaderir/glsl` kennt genau zwei Versionen und emittiert `#version
150` oder `#version 300 es`. In GLSL ES 3.00 sind Ableitungen Kernsprache. Die
Suche nach `ES100`, `#version 100` und `OES_standard_derivatives` im ganzen
Modul liefert nichts.

Der Shader kommt in WU-E trotzdem ohne Ableitungen aus, aber aus anderen
Gruenden, und der damals befuerchtete Ausweichweg war ebenfalls ein Irrtum: es
braucht keinen zusaetzlichen Vertex-Slot und keinen zweiten Shader. Die
Lokal-nach-Device-Skalierung ist auf der CPU bekannt und wird in die fuenf
ohnehin uebertragenen Werte eingerechnet. Danach ist das Distanzfeld in
Device-Pixeln gemessen, die AA-Breite ist konstant 1, und `fwidth` entfaellt.
Das hat nebenbei einen echten Fehler behoben - die Vertex-Polsterung war in
lokalen Einheiten und schnitt unter Verkleinerung die aeussere Haelfte jeder
geglaetteten Kante ab - und die Artefakte an den Knicken des Distanzfeldes
beseitigt.

**Das tatsaechliche Pi-Risiko** ist, ueberhaupt keinen ES-3.0-Kontext zu
bekommen. Das betraefe den gesamten Renderer und nicht nur abgerundete Formen,
und kein Shader-Umbau hilft dagegen. Pi 4 und Pi 5 koennen es mit Mesa v3d;
zu pruefen bleibt es trotzdem, aber als gewoehnlicher Plattformtest und nicht
als vorgezogenes Designrisiko.

Lehre fuers Vorgehen: diese Passage stand drei Revisionen lang im Plan, weil
eine plausible Behauptung eines Agenten ungeprueft uebernommen wurde. Fuer
Aussagen ueber Fremdcode gilt ab sofort dasselbe wie fuer Messwerte - Beleg
aus der Quelle oder sie stehen nicht im Plan.

Groesster Einzelposten des Projekts. Shaping-Anbindung an go-text/typesetting,
Messung, Shaping-Cache, Glyph-Atlas und Eviction. Danach Button-Interaktion,
Fokus und das gemeinsame Pointer-Modell inklusive Touch. Border, Radius und
gecachte Shadows integrieren; Paint-Bounds und Clip-Semantik testen.

Ergebnis: example-counter und erweiterte Layout-Demo, einschliesslich
Tastatur- und Touch-Bedienung. Noch kein Texteditor, kein Bidi, kein
Font-Fallback.

Praezisierung nach Review-Gate 6: die beiden sind **nicht** zwei Beispiele.
Der Counter ist das Beispiel und heisst auch so; die belastbare Layout-Demo
ist `internal/stress` mit `cmd/gift-stress`, wo Parametrisierung der Zweck ist
und headless-Assertions moeglich sind. Ein zweites Schaubeispiel, das nur
Rechtecke zeigt, waere fuer einen Leser wertlos gewesen - das war der
urspruengliche Zustand von `example-layout` und der Grund, es zu ersetzen.

### Schritt 3: Galerie ohne I/O

100.000 synthetische Metadatensaetze, virtuelle Masonry- und anschliessend
Justified-Layouts, Platzhalter, Scrollanker, Selektion, kinetisches Scrollen und
Recycling umsetzen.

Ergebnis: example-gallery kann deterministisch scrollen, springen, umkehren,
resizen und Layouts wechseln. Der reine Scrollpfad ist unabhaengig von N
abgesehen von der Indexsuche.

### Schritt 4: Vollstaendige Image-Pipeline

File-/HTTP-Source, Metadaten, Orientierung, Thumbnail-Erzeugung, Disk-/CPU-/GPU-
Caches, Abbruch und Upload-Budgets integrieren. ui.Image und Galerie teilen
Ressourcen. Zunaechst JPEG und PNG mit dokumentierter Orientierungsbehandlung;
weitere Formate nur ueber klar registrierte Decoder. Kein RAW-/Video-Support.

Ergebnis: example-gallery zeigt reale lokale und HTTP-Bilder, inklusive
Cold-Loading, Fehlern und dauerhaftem Thumbnail-Cache.

### Schritt 5: Glass und integrierter erster Aufschlag

Materialregionen in der Display-Liste, Reduced-Material als
Einzelpass-Shader, danach Full-Material mit Regionskopie und
Dual-Kawase-Kette. Messungsbasierte Adaptive-Politik mit Hysterese.
example-effects zeigt Border, Shadow und ein Glass-Panel ueber der Galerie.

Erst nach diesem Schritt ist der hier vereinbarte erste Aufschlag vollstaendig.
Eine isolierte Counter-Demo gilt nicht als Erfuellung dieses Plans.

## 13. Tests, Budgets und Review-Kriterien

### Paket `gifttest`: Testwerkzeug fuer Anwender

Erweiterung ueber den urspruenglichen Plan hinaus, beauftragt nach Schritt 2.
Dieser Abschnitt regelte bisher nur, wie Gift **sich selbst** testet. `gifttest`
ist das Gegenstueck fuer Anwendungen, die auf Gift aufbauen, im Geist von
Playwright.

- **Headless ist der Normalfall.** Layout, Paint und Assertions laufen auf der
  Display-Liste, ohne GPU und ohne Fenster, also in gewoehnlicher CI mit
  `go test ./...`. Pixelvergleiche sind optional hinter `giftgpu`.
- **Selektoren statt Koordinaten.** `ByText`, `ByKey`, `ByType`, kombinierbar.
  Koordinatengebundene Tests sind genau das, was vermieden werden soll. Dafuer
  traegt `gift.Element` ein `Label`-Feld; das ist bewusst im Runtime-Vertrag
  und nicht in `ui`, weil ein Selektor ueber den ganzen Baum laufen muss und
  eine spaetere Accessibility-Bruecke dieselbe Information braucht. Es kostet
  im Frame-Pfad nichts und ist so gemessen.
- **Deterministische Zeit.** `App.BeginInput(now)` nimmt die Zeit als Parameter,
  also sind Long-Press und spaeter Animationen ohne `time.Sleep` testbar.
- **Fehlermeldungen sind das eigentliche Produkt.** Ein mehrdeutiger Selektor
  nennt alle Treffer mit Bounds, markiert sie im Baumauszug und nennt den
  unterscheidenden Vorfahren - der zugleich die Loesung ist. Ein Null-Treffer
  nennt die tatsaechlich vorhandenen Labels, Keys oder Typen. `Settle` bricht
  eine Endlos-Rebuild-Schleife mit Diagnose ab, statt zu haengen.
- **Golden-Bilder** mit Toleranz 4 je Kanal und **ohne Ausreisser-Budget.** Ein
  Prozentsatz erlaubter Abweichung ist genau der Mechanismus, der eine
  verschobene Form durchwinkt: eine versetzte Kante trifft nur die Pixel entlang
  einer Linie. Geprueft: zwei Pixel Versatz erzeugen 4,1 % abweichende Pixel bei
  maximaler Differenz 255. Bei Fehlschlag werden Ist-Bild und Diff geschrieben
  und benannt, bei Erfolg wieder entfernt.
- **Strukturelle Assertions haben Vorrang vor Bildern.** Ein Golden sagt "das
  hat sich geaendert", nie "das ist falsch", und laeuft im Normalbuild gar
  nicht. Assertions auf der Display-Liste sagen warum.
- **Aktionen pruefen ihr Ziel.** Eine Aktion rechnet sich den Punkt aus den
  Bounds des Knotens aus und steigt zum naechsten interaktiven Vorfahren auf,
  damit ein Test nicht die Interna eines Widgets festschreibt. Danach wird per
  Hit-Test geprueft, **ob der Klick auch dort ankommt**. Review-Gate 3 hat
  belegt, dass ohne diese Pruefung ein Test einen verdeckten Button anklicken
  und trotzdem gruen sein kann. Der bewusste Klick durch etwas hindurch ist
  `ClickAt` mit expliziter Koordinate.

### Bekannte Falle: `t.Parallel` und prozessweiter Zustand

Benannt, nicht behoben, mit Absicht. `internal/text` haelt einen prozessweiten
Shaper samt Shaping-Cache, und `ui.SetDefaultFont` ist prozessweit
veraenderlich. Solange kein Test `t.Parallel` aufruft, ist nichts kaputt; das
Repository enthaelt heute keinen einzigen.

Warum nicht jetzt beheben: ein Mutex saesse auf dem Messpfad, der im Layout
laeuft und damit im 0-B/op-Vertrag aus Abschnitt 11 - und braechte einer
Einfenster-Anwendung nach Abschnitt 1 nichts. Die saubere Loesung ist ein
Shaper je `App`, und die scheitert daran, dass der Layout-Vertrag einem
Layouter ein `*LayoutContext` und keinen Anwendungs-Handle gibt. Das ist eine
Aenderung an `gift.Layouter`, keine Nachbesserung.

Ausloeser zum Beheben: sobald eine zweite `App` im selben Prozess gebraucht
wird, etwa ein Vorschaufenster in Schritt 3. Dann ist es ein
Korrektheitsproblem und kein Testkomfortproblem, und der Shaper je `App` wird
richtig gebaut statt mit einem Lock geflickt. Bis dahin gilt Abschnitt 15:
fuer nebenlaeufige Fehler ist `-race` das verbindliche Werkzeug.

Automatisierte Korrektheitstests:

- State-Key-Isolation, Unmount, geaenderte Abhaengigkeiten und stale Async-Resultate.
- Slice-Ownership-Verletzung wird im Debug-Build erkannt.
- Stack-Constraints, Text-Baselines, Zeilenumbruch, Modifier-Zeichenreihenfolge
  und Paint-Bounds.
- Pointer-Capture, Fokus, Disabled, Scroll-/Clip-transformiertes Hit-Testing,
  Touch-Drag gegen Tap-Abgrenzung.
- Galerieindex gegen eine einfache Vollsuche, inklusive Random-Jumps und Resize.
- Keine falschen Bilder nach Tile-/Slot-Recycling und Quellenrevisionen.
- Bytebudgets und Freigabe bei Fehler, Abbruch, Queue-Saettigung und Shutdown.
- HTTP-Validatoren, defekte/zu grosse Bilder, unbekannte Abmessungen und Orientierung.
- Shadow-/Glass-Invalidierung bei bewegtem Hintergrund; Full-/Reduced-Fallback
  inklusive Hysterese ohne Flackern.

Build-/Benchmark-Pruefungen:

- go test und go vet fuer die implementierten Packages; Race-Tests fuer CPU-Pipeline.
- CGO_ENABLED=0 fuer lokalen Desktop und Linux/arm64-Cross-Build.
- Tests des Kerns ohne Fenster oder GPU; Renderer-Tests separat mit Grafik-Kontext.
- Allokationsbenchmarks nach Warmup fuer Scrollen, Indexsuche und Transformationen.
- Allokationsbenchmark des Frame-Pfads mit aktivem Debug-Level-Logger: 0 B/op.

### Abnahmeschwellen

Referenz ist Pi 4, Raspberry Pi OS 64-Bit, 1920x1080 bei 60 Hz, fixierte
Qualitaetsstufe, ohne Throttling. Durchschnitts-FPS ist kein Kriterium.

Definition "verpasstes Intervall", festgelegt in WU-D. Das nominale Intervall
bei 60 Hz ist 16,667 ms. Ein reales Display trifft das nie exakt, und ein
strikter Vergleich gegen 16,67 ms hat in einer sauber getakteten Messung
50 % der Frames als verpasst gemeldet. Verbindlich ist deshalb eine Toleranz
von **0,5 ms**: ein Intervall gilt als verpasst, wenn es **17,17 ms**
ueberschreitet. Nominalwert, Toleranz und daraus folgende Schwelle werden in
jeder Messausgabe mitgefuehrt, damit die Zahl nachvollziehbar bleibt.

Ebenfalls verbindlich, ergaenzt in WU-C2, weil beides `missed_ratio`
materiell verschiebt und eine Messung sonst nicht zwischen Laeufen
vergleichbar ist:

- **Aufwaermen.** Die ersten 60 Intervalle werden verworfen. Das Oeffnen des
  Fensters erzeugt auf der Referenzmaschine ein Intervall von 123 bis 148 ms;
  bei 4096 Ringplaetzen kann ein 60-Sekunden-Szenario diesen Ausreisser nie
  verdraengen. Die Zahl ist auf der Zielhardware einmal nachzumessen, dort ist
  das Aufwaermen vermutlich laenger.
- **Sub-Frame-Intervalle.** Intervalle unter 1 ms sind zwei `Draw`-Callbacks
  direkt hintereinander, keine zwei Praesentationen. Sie werden verworfen und
  **getrennt gezaehlt**. Stilles Verwerfen waere derselbe leise Optimismus wie
  eine falsch gerundete Perzentile.
- Verworfene Aufwaermframes und Sub-Frame-Intervalle stehen als eigene Zahlen
  in jeder Messausgabe.

Perzentile werden nach Nearest Rank gebildet, nicht kaufmaennisch gerundet.
Kaufmaennisches Runden lag in 282 von 900 geprueften Faellen eine Probe zu
niedrig, also systematisch optimistisch genau am Tail, ueber den das Kriterium
entscheidet. `p99,9` ist bei 60 Hz erst ab etwa 1000 Proben aussagekraeftig und
entartet darunter zum Maximum; das ist in der Ausgabe kenntlich zu machen.

| Szenario | Kriterium |
| --- | --- |
| Scroll 60 s, 100k Platzhalter | p99-Frametime < 16,67 ms; < 1 % verpasste Intervalle |
| Scroll 60 s, warme reale Bilder | p99 < 16,67 ms; < 2 % verpasste Intervalle |
| Kalter Cache, Scroll 60 s | p99,9 < 33 ms; keine Stalls > 100 ms |
| Frame-Pfad ohne Build, aufgewaermt | 0 B/op im Allokationsbenchmark |
| Build eines Counter-Scopes | 0 Allokationen im Anteil von gift; siehe unten |
| Glass Reduced ueber Galerie | Zusatzkosten < 1,0 ms je Frame |
| Glass Full ueber Galerie | Zusatzkosten < 4,0 ms je Frame; Materialflaeche <= 25 % des Screens |
| Shadow, 20 sichtbare Instanzen | Zusatzkosten < 0,5 ms je Frame nach Cache-Aufwaermung |
| Langes Scrollen, 10 min | RSS-Plateau; Wachstum < 5 % ueber die letzten 5 min |
| GPU-Bildspeicher, 100k Eintraege | geschaetzt < 256 MiB resident |

Pi 5 wird getrennt gemessen und darf strengere Werte erreichen; er ersetzt die
Pi-4-Abnahme nicht. Auf Pi 4 ist `Glass Full` ausdruecklich als "darf die
Schwelle verfehlen" gekennzeichnet; dann greift Reduced als Default und der
Fehlschlag wird dokumentiert, nicht versteckt.

Korrektur zur Build-Schwelle, belegt in WU-B2. Die urspruengliche Vorgabe
"< 8 Allokationen pro Build" war nicht erreichbar und ist zurueckgenommen. Der
Counter aus Abschnitt 4 hat einen strukturellen Boden von neun Allokationen,
die der Aufrufer erzeugt, bevor gift beteiligt ist: zwei variadische
Kinderslices, fuenf Boxings von Kindwerten in `View` und zwei Closures fuer die
Buttons. Das ist Variante A und deckt sich mit Abschnitt 11, der Build
ausdruecklich vom 0-B/op-Vertrag ausnimmt. Gemessen wird deshalb der Anteil von
gift selbst; er muss 0 sein. Die Gesamtzahl inklusive View-Konstruktion wird
danebengestellt und beobachtet, aber nicht als Schwelle gefuehrt.

example-gallery und example-effects erhalten reproduzierbare Szenarien und
maschinell lesbare Messausgaben. Erfasst werden Build-/Layout-Aufrufe, sichtbare
Nodes, Uploadbytes, Queuealter, Cache-Hits, Platzhalterdauer, Allokationen, Heap,
RSS soweit verfuegbar und geschaetzter GPU-Bildspeicher. CPU-Zeiten,
Draw-/Swap-Intervalle und echte GPU-/Praesentationszeiten werden nicht vermischt.

Auf der Zielhardware getrennt pruefen:

| Szenario | Aussage |
| --- | --- |
| 10k / 100k Platzhalter | Kosten des Index und der Virtualisierung |
| Warme reale Bilder | Rendering, Batching und Cache-Stabilitaet |
| Kalter Thumbnail-/Datei-/HTTP-Cache | Decode, I/O, Upload-Stalls, GC und Nachladezeit |
| Schnelle Spruenge und Richtungswechsel | Priorisierung, Abbruch und Recycling |
| Resize und Layoutwechsel | Reflow, Scrollanker und alte Layoutresultate |
| Border / Shadow / Reduced Glass / Full Glass | Isolierte Effektkosten |
| Langes Scrollen, Minimieren, Wiederherstellen | Speicherplateau und Lifecycle |

Mesa, Display-Stack, RAM, Datentraeger, Aufloesung, Skalierung und
Temperatur/Throttling protokollieren. Eine Budgetverletzung muss sichtbar sein;
keine stillschweigende Verringerung der Bildanzahl oder Effektqualitaet im
Benchmark.

## 14. Bewusst nicht im MVP

Damit der Umfang nicht unbemerkt waechst, ausdruecklich ausgeschlossen:

Bidi/RTL, vertikale Schrift, Font-Fallback-Ketten, IME im Sinne der
CJK-Komposition, Subpixel-Positioning, Multiwindow, native
Accessibility-Bridges, Pinch-Zoom und Rotation, RAW- und Videoformate,
Fraktionale DPI-Skalierung, Internationalisierung der Beispiele,
On-Demand-Rendering, eigene Atlas-Engine, Signal-/Effect-Graph.

Fehler-/Logging-Strategie und API-Stabilitaet sind in Abschnitt 15 geregelt.

### Aufgehobene Ausschluesse

Der erste Aufschlag ist abgeschlossen. Der Auftraggeber hat danach fuenf
Punkte angefordert, die hier ausgeschlossen waren. Diese Liste beginnt mit
dem Satz, dass der Umfang nicht unbemerkt wachsen soll; also wird das
Wachstum hier bemerkt und begruendet, statt im Code zu passieren.

- `Texteditor` faellt. Begruendung: Abschnitt 19. Was bleibt, ist `IME` im
  engeren Sinn, also Kandidatenfenster und Komposition fuer CJK. Umlaute,
  AltGr und Dead Keys sind kein IME, sondern die normale Uebersetzung von
  Tastendruecken in Zeichen, die jede Tastatur ausserhalb der USA braucht.
  Der bisherige Text hat beides in einen Topf geworfen; das war falsch.
- `Theming/Dark-Mode-System` faellt, aber nur zur Haelfte: Abschnitt 20
  fuehrt semantische Farben und einen Hell-/Dunkel-Umschalter ein. Ein
  `System` im Sinne vererbter Umgebungswerte entsteht ausdruecklich nicht,
  weil Abschnitt 4 ab Zeile 189 dagegen argumentiert und dieses Argument
  weiter gilt.
- `Fraktionale DPI-Skalierung` bleibt ausgeschlossen und wird praezisiert.
  Ausgeschlossen war nie die Geraetedichte als solche, sondern der
  gebrochene Faktor. Ganzzahlige Dichte war nirgends verboten und nirgends
  vorgesehen; sie war eine Luecke, die dieser Ausschluss zugedeckt hat.
  Abschnitt 18 schliesst sie.
- `Animationskurven jenseits einfacher Interpolation` faellt. Das
  Liquid-Glass-Beispiel braucht sie, und Abschnitt 8 hat die Kostenseite von
  Glass bereits vermessen.
- `Font-Fallback-Ketten` bleibt ausgeschlossen. Abschnitt 17 bringt ein
  Font-Register, das ist nicht dasselbe: das Register waehlt eine Schrift
  anhand von Familie, Gewicht und Stil, es setzt keine zweite Schrift fuer
  fehlende Glyphen ein.
- `Pinch-Zoom und Rotation` faellt. Fuer den Bildbetrachter des
  Folgeprojekts ist ein zoombares Bild mit Vollbildansicht angefordert;
  Abschnitt 26 haelt Recherche, Entscheidungen und Messungen fest.
- `Videoformate` faellt, `RAW` bleibt ausgeschlossen. Fuer ein Folgeprojekt
  ist ein integrierter Videoplayer mit Ton angefordert. Abschnitt 25 haelt
  Machbarkeit, Messungen und Architektur fest; `render.OpVideo`,
  `render.Videos`, `render.AudioOutput`, das Paket `video` und
  `ui/videoplayer` setzen sie um.

Nicht aufgehoben, sondern nie enthalten: Zwischenablage und
Bildschirmtastatur kommen in diesem Dokument bis hier nicht vor. Sie sind
in Abschnitt 19 geregelt.

## 15. API-Stabilitaet, Fehler und Logging

### API-Stabilitaet

Die API ist **vorerst nicht stabil**. Es gibt keine Deprecation-Frist, keine
Migrationshilfen und keine Zusage zu Signatur-, Namens- oder
Verhaltenskonstanz. Breaking Changes duerfen jederzeit und ohne Ankuendigung
erfolgen, auch innerhalb eines Schrittes.

Das Modul bleibt auf v0. Es wird kein `/v2`-Pfad und kein Stabilitaetsversprechen
in der Dokumentation behauptet.

Der Uebergang zu stabiler API erfolgt **ausschliesslich auf ausdrueckliche
Ansage des Projekteigners**, nicht automatisch nach Abschluss von Schritt 5 und
nicht abgeleitet aus dem Reifegrad. Bis dahin ist "das hat sich geaendert" kein
Fehlerbericht. Ab dieser Ansage wird dieser Abschnitt durch eine konkrete
Kompatibilitaetspolitik ersetzt.

### Fehlerbehandlung

- Programmierfehler im Vertrag von Gift werden mit Panic und klarer Diagnose
  abgewiesen, nicht stillschweigend toleriert: Typwechsel unter gleichem
  State-Key, Verletzung des Slice-Ownership, State-Zugriff ausserhalb des
  UI-Executors, Build waehrend `Draw`.
- Nicht jede dieser Pruefungen ist im Release-Build aktiv. Typwechsel, Build
  waehrend `Draw` und Re-Entranz panicken immer. Slice-Ownership,
  Duplikat-Keys und die UI-Executor-Pruefung sind nur unter dem Build-Tag
  `giftdebug` aktiv, weil ihre Erkennung sonst den 0-B/op-Vertrag aus
  Abschnitt 11 im Eventhandler-Pfad verletzen wuerde. Fuer nebenlaeufige
  Fehler ist ohnehin `-race` das verbindliche Werkzeug, nicht diese
  Laufzeitpruefung.
- Laufzeitfehler aus der Aussenwelt sind normale Werte: I/O, HTTP, Decode,
  Cache. Sie werden ueber den asset-Vertrag zurueckgegeben und fuehren zu einem
  sichtbaren Fehlerzustand der betroffenen Kachel, nie zum Abbruch des Frames.
- Ein Fehler erzeugt keinen Retry-Sturm. Fehlgeschlagene Quellen bekommen einen
  Backoff und werden bis zur Revisionsaenderung nicht erneut geladen.
- **Vorlaeufig gegenueber dauerhaft ist ein Typ, kein Kommentar.** Nachtrag aus
  Review-Gate 5. Die Pipeline unterscheidet sorgfaeltig zwischen Fehlern, die
  ein erneuter Versuch behebt (Saettigung, Backoff), und solchen, die er nicht
  behebt (kein Bild, zu gross, kaputte URL) - und reichte dem Konsumenten dann
  ein blankes `error` in einem Feld. **Beide** Konsumenten haben daraufhin eine
  voruebergehende Ablehnung dauerhaft gemerkt: ein schneller Scroll vergiftete
  die Queue, die Queue lehnte ab, und die Kachel blieb fuer immer leer.

  Konsequenz: `Result` hat kein Feld mehr, das "ein Fehler" bedeutet. Es hat
  `Retry` und `Failure`, und beide Namen sagen, was zu tun ist. `Err` ist eine
  Methode, sodass `if res.Err != nil` nicht mehr durch `go vet` kommt. Die
  Regel dahinter gilt ueber diesen Fall hinaus: **wo zwei Pakete sich ueber die
  Bedeutung eines Fehlers einig sein muessen, gehoert die Unterscheidung in den
  Typ und nicht in die Dokumentation.** Ein dritter Konsument soll den Fehler
  nicht wiederholen koennen, nicht bloss davor gewarnt werden.

  Nachtrag dazu: nicht jeder Backoff ist voruebergehend. Eine unter Quarantaene
  stehende Quelle liefert Backoff und kommt ohne Revisionsaenderung nie wieder
  frei. Waere die als wiederholbar eingestuft worden, haette ein 404 in einem
  stehenden Fenster eine abgelehnte Anfrage und ein Relayout **je Frame,
  dauerhaft** erzeugt - eine leere Kachel gegen eine Endlosschleife getauscht.
  Deshalb ist Quarantaene ein eigener, endgueltiger Fehler.
- Gift ruft in keinem Pfad `os.Exit` oder `log.Fatal`.

### Logging

Regel: **Logging darf die Performance nicht beeinflussen.** Daraus folgt eine
harte Trennung.

**Frame-Hotpath: kein Logging.** In `Update` und `Draw` wird nicht geloggt, auch
nicht auf Debug-Level und auch nicht hinter einer Levelpruefung. Der Grund ist
nicht nur die Ausgabe, sondern die Konstruktion der Attribute: jedes
`slog.Any`/`slog.String` im Hotpath erzeugt Allokationen und verletzt den
0-B/op-Vertrag aus Abschnitt 11. Eine Verletzung faellt im Allokationsbenchmark
des Go/No-Go auf.

**Stattdessen: Zaehler.** Der Frame-Pfad schreibt ausschliesslich in eine
vorallozierte Diagnostik-Struktur mit einfachen numerischen Feldern, ohne
Formatierung, ohne Interface-Boxing, ohne Locks im Normalfall. Sichtbare Nodes,
Uploadbytes, Cache-Hits, verpasste Intervalle und die effektive Glass-Stufe sind
Zaehler, keine Logzeilen.

Die Zaehler werden ausserhalb des Hotpaths abgegriffen: entweder durch die
Anwendung per Snapshot-Methode oder durch einen optionalen Diagnose-Sink mit
niedriger Rate, hoechstens einmal pro Sekunde. Nur dort entsteht ein
`slog.Record`. Die maschinenlesbaren Messausgaben aus Abschnitt 13 speisen sich
aus diesem Snapshot, nicht aus Logparsing.

**Ausserhalb des Hotpaths: `log/slog`.** Lifecycle, Asset-Pipeline, Cache,
Backend-Initialisierung und Fehlerpfade verwenden slog. Fuer Domain- und
Anwendungscode ist slog verbindlich; Gift gibt keine eigene Logger-Abstraktion
und kein eigenes Logger-Interface vor.

- Gift loggt nie nach `slog.Default()`. Der Logger wird bei der App-Konstruktion
  uebergeben; ohne Angabe ist Logging aus, nicht "Default".
- `context.Context` wird in der asset-Pipeline durchgereicht, damit die
  Anwendung ihren Handler mit Requestbezug anreichern kann.
- Kosten entstehen nur bei aktivem Level: teure Attribute werden hinter
  `Logger.Enabled` oder `slog.LogValuer` gebildet, nie unbedingt.
- Keine Zugangsdaten, Tokens oder vollstaendigen authentifizierten URLs in
  Logs oder Cache-Dateinamen, siehe Abschnitt 9.
- Levelgebrauch: `Error` nur fuer Fehler, die der Anwender sehen muss;
  `Warn` fuer degradierte Qualitaet, etwa Rueckfall auf Glass Reduced;
  `Info` fuer Lifecycle und Konfiguration; `Debug` fuer Pipeline-Details.
  Kein `Info` pro geladenem Bild bei 100.000 Eintraegen.

Getestet wird: ein Allokationsbenchmark des Frame-Pfads mit aktivem
Debug-Level-Logger muss weiterhin 0 B/op liefern.

## 16. Im Review zu bestaetigen

1. Modulpfad `github.com/worldiety/gift`, Go 1.27, Ebitengine 2.10.1.
2. Frame-Modell: Vollredraw jedes Frames, Invalidierung spart CPU-Arbeit und
   nicht Fuellrate, kein On-Demand-Rendering.
3. Package-Grenzen: kleine Modulwurzel fuer Runtime-Vertraege, ein ui-Package,
   eigenstaendiges asset und render, genau ein konkretes Backend.
4. Style-Semantik: feste Zeichenreihenfolge statt positionsabhaengiger SwiftUI-
   Modifier-Wrapper im ersten Aufschlag. Slice-Ownership geht an Gift ueber.
5. Effekte: Border/Shadow regulaer, Glass Reduced als Default-Einzelpass,
   Glass Full experimentell mit Regionskopie und Kawase-Kette, Adaptive
   messungsbasiert. Keine Apple-Pixelparitaet, keine Full-Glass-60-fps-Zusage.
6. Text: go-text/typesetting, LTR ohne Bidi und ohne Font-Fallback im MVP.
7. Eingabe: Maus, Tastatur und einfache Touch-Gesten inklusive kinetischem
   Scrollen gehoeren in den ersten Aufschlag.
8. Galerie: Masonry und Justified/Brick, automatische lokale/HTTP-Bildpipeline,
   Platzhalter als bewusstes Verhalten bei fehlenden Bildern.
9. Abnahmeschwellen aus Abschnitt 13 und das Go/No-Go nach Schritt 1.
10. Plattformumfang und Ausschlussliste aus Abschnitt 14.
11. API vorerst instabil, Stabilisierung nur auf ausdrueckliche Ansage.
    Kein Logging im Frame-Hotpath, Zaehler statt Logzeilen, `log/slog`
    ausserhalb des Hotpaths und verbindlich im Domain-Code.

## 17. Fonts und Font-Register

Die Beispiele suchen heute jeweils mit einer eigenen, dreimal kopierten
`loadFont` vier Systempfade ab und fallen auf einen CWD-relativen Pfad
zurueck. Das ist aus drei Gruenden zu ersetzen, und nur der erste ist
offensichtlich:

1. Auf einem minimalen Pi-OS-Image greift kein einziger Pfad.
2. Die *gewaehlte* Schrift unterscheidet sich je Plattform, also ist jedes
   Pixelergebnis der Beispiele konstruktionsbedingt maschinenabhaengig.
3. `gifttest` kann Anwendern keine stabilen Goldens anbieten. Die Testschrift
   liegt unter `internal/` und wird ueber einen relativen Pfad geladen; beides
   ist ausserhalb dieses Moduls nicht erreichbar.

### Register statt Einzelslot

`ui.SetDefaultFont` setzt eine einzige globale Variable. Zwei
Side-Effect-Importe wuerden einander in nicht festgelegter
`init`-Reihenfolge lautlos ueberschreiben. Deshalb ein Register, das eine
Schrift unter Familie, Gewicht und Stil aufnimmt, mit `gift.RegisterType` als
Vorbild fuer die Registrierung zur Paketinitialisierung.

Das Register loest *aus den registrierten Schriften* auf. Es ist keine
Fallback-Kette: fehlt ein Glyph in der gewaehlten Schrift, wird keine zweite
Schrift befragt. Abschnitt 14 haelt diesen Ausschluss aufrecht.

### Mitgelieferte Schriften

Zwei optionale Pakete, die nichts kosten, solange sie nicht importiert
werden. Das ist dasselbe Argument, das `internal/stress` fuer seine
eingebettete Schrift bereits fuehrt.

- Inter, statische Instanzen aus dem Upstream-Release v4.1.
- IBM Plex Mono.

Belegte Einschraenkung, die die Quelle der Schriften bestimmt:
`go-text/typesetting` v0.3.5 erkennt in `font/opentype/reader.go` die
Signatur `wOFF`, aber nicht `wOF2`. Die Schriften im untersuchten
Nago-Stand liegen fuer Inter ausschliesslich als WOFF2 vor (Magic
`774f4632`); IBM Plex Mono liegt dort zusaetzlich als WOFF1 vor. Inter wird
deshalb nicht aus dieser Quelle uebernommen, sondern als TTF vom Upstream
bezogen. Ein WOFF2-Decoder in Go waere Brotli plus die
glyf-Ruecktransformation und steht in keinem Verhaeltnis zum Nutzen.

`gifttest.Options` erhaelt ein Font-Feld, damit ein Golden nicht mehr davon
abhaengt, was irgendein anderer Test zuletzt global gesetzt hat.

## 18. Geraetedichte

Die Zielplattform ist ein Pi bei 1920x1080, also Dichte 1. Auf einer
Entwicklungsmaschine mit Retina-Display ist die Ausgabe sichtbar unscharf.
Die Ursache ist nicht kosmetisch:

`backend/ebiten/run.go` implementiert `Layout(int, int) (int, int)` und gibt
die Groesse unveraendert zurueck. Ebitengine rendert damit in ein Bild in
logischer Groesse und skaliert es gefiltert auf den physischen Framebuffer.
`DeviceScaleFactor` wird nirgends aufgerufen. Glyphen rastern bei
Punktgroesse, und der Renderer dokumentiert die Annahme, der Massstab sei
eins, mit `FilterNearest` fuer Glyphen.

Verbindlich:

- `LayoutF` statt `Layout`, Dichte aus `ebiten.Monitor().DeviceScaleFactor()`
  beim Start und bei Monitorwechsel.
- Ein Dichte-Transform an der Wurzel der Display-Liste. Die Shaderseite ist
  darauf vorbereitet: `deviceScale` backt Radius, Strichbreite, AA-Rand,
  Schattensigma und Refraktion bereits aus dem Transform.
- Glyphen rastern in Geraetepixeln. Der Atlas-Key traegt bereits eine
  Groesse; er traegt sie dann in Geraetepixeln.
- Die Bildleiter waehlt ihre Sprosse in Geraetepixeln. Heute waehlt sie nach
  der Layoutgroesse in Punkten, ein Foto ist also auf einem 2x-Display
  unabhaengig vom Framebuffer-Problem um den Faktor zwei unterversorgt.

Ganzzahlige Faktoren sind zugesagt. Gebrochene Faktoren bleiben nach
Abschnitt 14 ausgeschlossen; trifft gift einen solchen, rundet es und
dokumentiert das Ergebnis, statt Genauigkeit zu behaupten, die es nicht hat.

Ein Steuerelement, das nur aus Rechtecken mit Radius besteht, rechnet selbst
**nicht** mit der Dichte. Die Skalierung steht in genau einem Transform an der
Wurzel der Display-Liste, und der Shader backt Radius, Strichbreite und
AA-Rand daraus. Ein Widget, das zusaetzlich runden wuerde, rundet zweimal.
Die Dichte betrifft nur, was gerastert wird: Glyphen, Icons und die
Bildleiter. Auch Layout und Zeigereingabe bleiben durchgehend logisch — eine
Trefferflaeche von 44 Pixeln ist bei jeder Dichte dieselbe Flaeche.

## 19. Texteingabe, Zwischenablage und Bildschirmtastatur

### Der Rune-Kanal ist die Voraussetzung

`backend/ebiten/input.go` fragt dreizehn fest verdrahtete physische
Tastencodes mit `IsKeyPressed` ab. Ebitengines eigene Dokumentation haelt zu
dieser Funktion fest, dass ein `Key` eine physische Taste des US-Layouts
bezeichnet, und stellt `AppendInputChars` als die
lokalisierungsabhaengige Uebersetzung nach Unicode daneben. gift ruft
`AppendInputChars` nirgends auf.

Die Folge ist nicht, dass Umlaute kaputt sind. Die Folge ist, dass **kein
druckbares Zeichen das Framework erreicht**. Ein perfektes `ui.TextField`
bekaeme null Zeichen. Der Rune-Kanal ist daher der erste Schritt und die
Voraussetzung fuer alles Weitere in diesem Abschnitt.

Dazu: `gift.Key` wird um Backspace, Delete und die fuer Kurzbefehle noetigen
Buchstaben erweitert. Tastenwiederholung baut gift selbst, weil Ebitengine
keine hat; das steht seit dem ersten Aufschlag in diesem Dokument, bisher
mit dem Zusatz, ein Texteditor braeuchte sie und sei ausgeschlossen. Der
Zusatz entfaellt, die Aussage bleibt.

### Zwischenablage ohne cgo

Ebitengine 2.10.1 hat keine oeffentliche Zwischenablage-API. Die
GLFW-Implementierung liegt unter `internal/` und ist nach den Importregeln
von Go dauerhaft unerreichbar. Eine externe Abhaengigkeit ist also
unvermeidlich; sie darf aber den cgo-freien Kern nicht brechen, weil
`CGO_ENABLED=0 GOOS=linux GOARCH=arm64` zur Abnahmematrix gehoert.

Deshalb purego, als optionales Paket per Side-Effect-Import. `purego` ist
bereits indirekte Abhaengigkeit ueber Ebitengine. purego fuehrt Linux auf
amd64 und arm64 als Tier 1 und bringt fuer `!cgo` einen eigenen
dlopen-Pfad mit; auf Darwin liegt ein vollstaendiger ObjC-Runtime bei.

- macOS: `NSPasteboard` ueber den ObjC-Runtime. Kein Eventloop, aber ein
  eigener OS-Thread-Zwang aus einem anderen Grund: `NSAutoreleasePool` ist
  thread-gebunden. `init` legt den Pool auf den Pool-Stack des **aufrufenden**
  Threads, `drain` nimmt ihn von demselben Stack. Eine Goroutine ist kein
  Thread und darf an jedem Preemption-Punkt wandern — und die purego-Aufrufe
  dazwischen sind solche Punkte. Ohne `runtime.LockOSThread` ist das ein
  seltener Speicherzugriffsfehler in libobjc mit `drain` als Program Counter.
  Beide Plattformhaelften sperren den Thread also, aus verschiedenen Gruenden:
  X11 wegen des blockierenden `XNextEvent`, macOS wegen dieser Bindung. Der
  Fehler trat vor der Behebung in etwa drei Prozent der Laeufe von
  `go test ./clipboard/ -count=3` auf und sah dadurch lange wie ein Gespenst
  aus; der Regressionstest erzwingt die Wanderung und macht daraus einen
  sicheren Fehlschlag.
- Linux/X11: `libX11.so.6` per dlopen, mit einer **eigenen**
  Display-Verbindung und einem unsichtbaren Fenster auf einem eigenen
  OS-Thread. Das ist keine Umstaendlichkeit, sondern X11: wer kopiert, wird
  Owner der `CLIPBOARD`-Selection und muss `SelectionRequest` bedienen,
  solange der Inhalt gelten soll. Ebitengines Verbindung ist dafuer nicht
  erreichbar.
- Windows: Best-Effort. Der Plattformumfang nach Abschnitt 1 kennt Windows
  nicht als Ziel.

Zwei Dinge werden nicht zugesagt: das INCR-Protokoll fuer grosse Transfers
entfaellt zunaechst, die uebertragbare Groesse wird gedeckelt und
dokumentiert. Und der Inhalt verschwindet unter X11 mit dem Prozessende;
das ist Standardverhalten und kein Fehler.

### Bildschirmtastatur

Ebitengine bietet weder IME noch eine Moeglichkeit, eine native
Bildschirmtastatur zu oeffnen. Auf dem Kioskziel existiert ohnehin keine.
gift zeichnet sie also selbst, als Overlay im vorhandenen `ZStack`,
eingeblendet auf `EventFocusGained`. Fuer das Freihalten des Eingabefelds
dient das bereits vorhandene und getestete `ScrollIntoView`.

Zwei Praezisierungen, nach der Umsetzung nachgetragen. Beide standen hier zu
knapp, und beide wurden beim Bauen gefunden, nicht beim Planen:

`ScrollIntoView` allein genuegt nicht, und zwar aus einem Grund der
Reihenfolge, nicht des Koennens. Wenn das Feld um Freistellung bittet, ist es
in der Eingabephase; die Tastatur wird erst danach im selben Frame gebaut und
vermessen. Die erste Freistellung hat also nichts, wovor sie ausweichen
koennte. Es braucht daher eine Kennzeichnung des verdeckenden Knotens und
eine nachgezogene zweite Freistellung. Das ist Verkabelung, damit der hier
genannte Mechanismus zum richtigen Zeitpunkt laeuft, und kein zweiter
Mechanismus.

Das Overlay kann ein Feld nicht freistellen, das die **letzte** Zeile eines
Formulars ist. Ein Container kann nur so weit heben, wie er noch scrollen
kann, und unterhalb der letzten Zeile ist kein Inhalt mehr. Ein
Content-Inset, der den Scrollbereich waehrend der Einblendung verlaengert,
waere die Loesung von iOS und Android; er greift aber in `ScrollInfo`, die
Proportionen des Scrollbalkens, die Overscroll-Verkettung und die
Fling-Begrenzung ein und ist damit nicht klein. Er bleibt vorerst
ausgeschlossen. Stattdessen gilt: das Overlay ist die richtige Anordnung,
wenn hinter der Tastatur kein Scrollcontainer liegt; liegt dort einer, ist
die Spaltenanordnung exakt, weil der Viewport dann wirklich schrumpft. Beide
Wege sind dokumentiert und getestet, und die Hoehe der Tastatur ist abfragbar,
damit eine Anwendung sich stattdessen Platz lassen kann.

Nachtrag, gefunden von einer Anwendung (Fotobox, 800x480, ohne
Hardwaretastatur): Die Ueberdeckung war der falsche Standard. Die Tastatur
nimmt dort mehr als die Haelfte des Fensters ein und verdeckte genau die
Bedienelemente am unteren Rand, die man beim Tippen braucht; jede Scrollansicht
brauchte einen Platzhalter in Tastaturhoehe. Ein `ZStack` (und damit
`ui.Window`), der ein `ui.OnScreenKeyboard()` als direktes Kind hat, legt
seine uebrigen Kinder deshalb jetzt standardmaessig im Bereich neben der
Tastatur aus, solange sie sichtbar ist ("adjustResize" von Android). Das ist
kein Content-Inset: der Viewport schrumpft wirklich, wie in der
Spaltenanordnung, und die zweite Freistellung ist dann exakt. Entschieden wird
im Layout anhand der gemessenen Tastaturhoehe; eine verborgene Tastatur misst
null und aendert kein Pixel. Die Fensterflaeche von `ui.Window` bleibt voll
gross. `Overlay.AvoidKeyboard(false)` stellt die Ueberdeckung wieder her; fuer
diesen Fall und fuer eine verpackte Tastatur bleibt die Verdeckungsmarkierung
mit ihrer oben beschriebenen Grenze bestehen. Ein `ui.Modal` gehoert in das
Fenster neben die Tastatur, dann liegt auch das Sheet oberhalb der Tastatur.

**Die Einblendung haengt an einem ausdruecklichen Kioskschalter, nicht an
`PointerKind`.** Begruendung, und sie ist belegt: Ebitengines Dokumentation
zu `AppendTouchIDs` haelt fest, dass die Funktion auf Desktops nichts tut;
dieser Umstand steht bereits in `backend/ebiten/input.go` und in
Abschnitt 7. Auf Raspberry Pi OS mit X11 meldet sich ein Touchscreen als
Maus. Eine an `PointerTouch` gebundene Einblendung wuerde auf genau der
Hardware nie ausloesen, fuer die sie gebaut wird.

## 20. Semantische Farben

Es gibt heute keine benannte Farbe. Jede Farbe ist ein Literal an der
Aufrufstelle, und jede Anwendung erklaert ihre Palette als Paketvariablen.

Eingefuehrt werden benannte Farben, die aus einem prozessweiten Thema
aufloesen, und ein Umschalter zwischen hell und dunkel zur Laufzeit.

Ausdruecklich **nicht** eingefuehrt wird ein Umgebungsmechanismus, der Werte
den Baum hinunterreicht. Abschnitt 4 ab Zeile 189 haelt fest, dass Styling
eine Eigenschaft der konkreten Konstruktionsstelle ist und nicht etwas, das
man auf eine beliebige View anwenden kann. Dieses Argument gilt
unveraendert. Ein Themenwechsel loest einen Neuaufbau ueber `App.Invalidate`
aus.

Eine Nebenwirkung, die mitgebaut werden muss: `ButtonStyle` kennt kein
"dieses Feld wurde nicht gesetzt", und Abschnitt 8 nennt genau das als Grund,
warum Zustandsstile ganze Boxstile ersetzen statt feldweise zu mischen. Ein
Thema laesst sich unter einen teilweise gesetzten Stil nur legen, wenn es
diese Unterscheidung gibt. Das Muster dafuer existiert bereits als
`hasFG` beim Textvordergrund.

## 21. Icons

Icons werden auf der CPU gerastert und als Bild gezeichnet. Es entsteht
**kein** Pfad-Primitiv in der Display-Liste. Begruendung, belegt:

- Der eine Shader ist ein Rounded-Box-SDF, und alle vier
  Vertex-Custom-Attribute sind belegt. Die Datei haelt selbst fest, dass ein
  fuenfter Wert bereits im Shader neu berechnet werden musste, weil kein
  Slot mehr frei war. Uniforms scheiden aus, weil sie in Ebitengine eine
  `map[string]any` sind und pro Operation allozieren wuerden.
- Ein zweiter Shader ist ein zweites Material und damit ein zusaetzlicher
  Draw-Call, also genau das, was der Ein-Shader-Entwurf vermeidet.
- Ein `OpPath` braeuchte eine Seitentabelle nach dem Vorbild der Glyphen und
  acht weitere Bytes im `Op`. Das waere die dritte Vergroesserung.

Der CPU-Weg kostet dagegen nichts Neues: `golang.org/x/image/vector` ist
bereits direkte Abhaengigkeit und wird in `internal/text` genau dafuer
verwendet. Eine Deckungsmaske mit Farbe erst zur Zeichenzeit ist dort
bereits das tragende Prinzip, und `OpImage.Color` ist ein Multiply-Tint,
also faerbt eine weisse Maske mal Vordergrundfarbe korrekt und
kantengeglaettet.

Die SVGs werden zur Bauzeit per `go:generate` zu Pfadsegmenten vorgeparst
und eingebettet. Damit entsteht kein SVG-Parser im Frame-Pfad und keine
neue Abhaengigkeit. Gerastert wird pro Groesse und zwischengespeichert; der
Frame-Pfad bleibt bei 0 B/op nach Abschnitt 11.

Der vorhandene Glyph-Atlas wird **nicht** wiederverwendet. Dieser Absatz hat
frueher das Gegenteil verlangt; die Umsetzung hat gezeigt, dass die Forderung
nicht erfuellbar ist, ohne dem heissesten Cache des Renderers einen Zweig
hinzuzufuegen. Der Atlas ist kein Deckungscache mit glyphfoermigem
Schluessel, sondern ein Glyphcache: sein Schluessel ist
`{FontID, Groesse, GlyphID}`, und sein Miss-Pfad schlaegt eine `*text.Font`
nach und rastert einen Umriss aus deren Tabellen. Ein Icon hat weder Font
noch Glyph-ID noch Umriss; es koennte den Atlas nur als Glyph einer Schrift
passieren, die es nicht gibt. Auch die Regalpackung ruht auf einer Annahme,
die Text erfuellt und Icons nicht: dass alle Glyphen einer Schrift in einer
Groesse aehnlich hoch sind.

Icons gehen deshalb durch `render.Images`, das genau diese Form von Problem
schon bedient: Residenz, ein Uploadbudget pro gezeichnetem Frame, Verdraengung
nach Alter und Bytes, und eine Generation, die ein verfallenes Handle
erkennbar macht statt ein falsches Bild zu zeigen. Abschnitt 14 bleibt
gewahrt, weil keine zweite Atlas-Engine entsteht; dreissig Zeilen Map auf
einem vorhandenen Dienst sind keine.

Der Preis gehoert dazu: eine Textur je Icon und Groesse statt einer
gemeinsamen Seite, also N Bildoperationen fuer N verschiedene Icons, die
nicht gebatcht werden. Bei Dutzenden Icons ist das tragbar, bei Tausenden
Glyphen waere es das nicht. Und weil das Uploadbudget pro gezeichnetem Frame
gilt, erscheint ein Schirm mit mehr neuen Icons als dem Budget erst ueber
mehrere Frames vollstaendig. Gemessen: acht pro Frame, also drei Frames fuer
zwanzig Icons. Ein Icon, dessen Upload abgelehnt wurde, zeichnet nichts und
holt es im naechsten gezeichneten Frame nach — es blitzt kein Platzhalter,
was fuer ein 16-Pixel-Symbol der schlechtere Fehler waere.

### Was `x/image/vector` nicht kann

Zwei Dinge, die dieser Abschnitt zunaechst verschwiegen hat, weil sie erst
beim Vermessen des Korpus sichtbar wurden. Beide sind der Grund, warum das
Paket groesser ist, als dieser Abschnitt vermuten liess.

**Der Rasterer fuellt, er strichelt nicht.** Die Haelfte des Korpus besteht
aus Strichen. Die Strichumrisse entstehen daher selbst, als Vereinigung
konvexer Stempel: ein Viereck je Segment, eine Verbindung je innerem Punkt,
eine Kappe je freiem Ende, alle gleich orientiert, sodass die
Nonzero-Fuellung genau ihre Vereinigung ist und keine Boolesche Operation
noetig wird. Ein versetzter Umriss wuerde sich selbst schneiden, sobald der
Pfad enger als die halbe Strichbreite dreht. Die Folge, die dazugehoert: die
Stempel ueberlappen, ein Strich laesst sich also nicht mit einer
transparenten Farbe zeichnen. Das kostet nichts, weil die Maske Deckung ist
und das Alpha in der Tinte steckt. Gestrichelt wird zur **Laufzeit**, nicht
vorab, weil der Umriss einer Kurve nur gegen eine Toleranz abgeflacht werden
kann und diese Toleranz von der Geraetepixelgroesse abhaengt, die der
Generator nicht kennt und der Cache-Miss sehr wohl.

**Der Rasterer kennt nur Nonzero.** Ein nennenswerter Teil des Korpus
deklariert `fill-rule="evenodd"`. Wie viele davon wirklich eine andere
Flaeche malen, ist zu **messen** und nicht zu schaetzen; gemessen waren es
45 von 211. Behoben wird das **offline**, indem Konturen nach Schachtelungs-
tiefe orientiert werden, und der Generator prueft jede Umorientierung gegen
die urspruengliche Evenodd-Fuellung, bevor er sie ausgibt.

Daraus das allgemeine Prinzip, das ueber diesen Fall hinausgeht und hier
festgehalten wird: **der Generator darf Arbeit tun, die der Frame-Pfad nicht
tun koennte, und alles, was der Frame-Pfad wiederholt tun muesste, gehoert in
den Generator.**

### Der Korpus, und was der Parser von ihm annimmt

Die Annahmen gehoeren aufgeschrieben, damit sie nachpruefbar sind, wenn der
Korpus sich aendert. Gemessen am Flowbite-Satz: **521 Dateien, 282 outline
und 239 solid**. Nur `<path>` und ein `<rect>`; kein `<g>`, keine
Transformationen, keine Verlaeufe. Keine Quadratiken. Der elliptische Bogen
ist mit Abstand der haeufigste Befehl und wird offline zu Kubiken. `solid/`
ist gefuellt, `outline/` gestrichelt mit Breite 2 und runden Kappen und
Verbindungen — wobei Miter der SVG-Standard ist und einige Pfade gar keine
Verbindung nennen, ihn also stillschweigend fordern.

Drei Ausnahmen, die je eine Regel im Code sind und ohne diese Notiz
willkuerlich aussaehen: ein abgerundetes `<rect>`, ein Pfad, der zugleich
gefuellt und gestrichelt ist, und ein Icon, dessen zweite Fuellung ein
literales Weiss ist und als Aussparung behandelt wird, weil ein Gift-Icon von
Bauart monochrom ist.

Icons gehen nicht durch `asset.Pipeline`. Die ist fuer Fotos gebaut, mit
Groessenleiter, Plattenspeicher-Cache und asynchroner Aufloesung; ein Icon
ist eingebettet, winzig und sofort verfuegbar, und ein einzelnes
Platzhalterbild fuer ein 16-Pixel-Symbol waere ein sichtbarer Fehler.

## 22. Weitere Bildformate

Abschnitt 12, Schritt 4 erlaubt weitere Formate ausdruecklich, sofern sie
ueber klar registrierte Decoder laufen. HEIF/HEIC ist weder RAW noch Video
und faellt daher nicht unter den Ausschluss in Abschnitt 14.

Eine Luecke im vorhandenen Vertrag muss dafuer geschlossen werden:
`asset/decode.go` liest die Orientierung nur fuer JPEG und nur aus EXIF. Ein
registrierter Decoder kann seine eigene Orientierung heute nicht melden.
HEIC traegt sie in den Boxen `irot` und `imir`. Es braucht daher eine
optionale Schnittstelle, ueber die ein Decoder seine Orientierung angibt.
Der Cache-Key traegt die Orientierung bereits, eine korrekt gemeldete
Orientierung wird also von selbst richtig zwischengespeichert.

`mimeByExtension` kennt nur `.jpg` und `.png` und wird ergaenzt. Da
Sniffing ohnehin Vorrang vor dem Medientyp hat, ist das die zweite
Verteidigungslinie und nicht die erste.

### Skaliertes JPEG mit libjpeg-turbo

Kamerafotos haben 12 bis 50 Megapixel. `image/jpeg` dekodiert jedes Pixel,
und die Pipeline wirft fuer eine Galeriekachel fast alle wieder weg; auf dem
Raspberry Pi kostet das pro Bild einen grossen Teil einer Sekunde.
`asset/turbojpeg` laedt libjpeg-turbo (`libturbojpeg0` auf Raspberry Pi OS)
zur Laufzeit ueber purego, ohne cgo, und benutzt nur die TurboJPEG-2-API,
weil Bookworm libjpeg-turbo 2.1 ausliefert.

Der Vertrag bekommt dafuer die optionale Schnittstelle `asset.ScaledDecoder`:
Die Pipeline fragt nach mindestens der Kachelgroesse im gespeicherten Raster,
der Decoder waehlt den kleinsten Faktor aus 1/8, 1/4, 1/2 und 1/1, der das
noch erfuellt, und der Resampler erledigt den Rest. Die uebrigen Achtel
bietet die Bibliothek auch an, sie laufen aber ohne SIMD und waren gemessen
langsamer als ein voller Decode. Das Dekodierbudget (Abschnitt 9) wird vom
Decoder selbst beziffert, weiterhin aus dem Bild und dem Codec-Risiko: Ein
progressives JPEG haelt alle Koeffizienten in voller Groesse, gleich wie
klein die Ausgabe ist, ein Baseline-JPEG nur Ausgabe und Zeilenpuffer.

Die Anmeldung bleibt ausdruecklich (Abschnitt 12, Schritt 4). gift nimmt
libjpeg-turbo nicht von selbst, nur weil sie installiert ist: Die Pixel
beider Decoder sind nicht bitgleich, und Vorschaubilder, die sich mit den
installierten Paketen aendern, sind schwerer zu testen als eine Wahl der
Anwendung.

### Rechtlicher Hinweis zu HEIF/HEIC

Das Format ist unerwuenscht. Es ist mit diversen Patenten belastet, und die
unlizenzierte Nutzung kann durch einander widersprechende
Patentverwerterpools abgemahnt werden. Wir liefern deshalb **keine
Implementierung mit**, ermuntern niemanden zur Nutzung und raten im
Gegenteil davon ab. Wer die Unterstuetzung braucht, bindet sie selbst ein
und traegt die Verantwortung dafuer. Wir uebernehmen keine Haftung. Dies ist
keine Rechtsberatung.

## 23. Zweite Lieferung in pruefbaren Schritten

Wie Abschnitt 12: jeder Schritt endet in etwas Vorzeigbarem, und jeder
Schritt wird unabhaengig geprueft. Die sechs Review-Gates der ersten
Lieferung haben jeweils mindestens einen Blocker gefunden, den der
Implementierer nicht selbst gemeldet hatte. Das Verfahren bleibt.

### Schritt 6: Zwei Fehler im Scrollen

`pointer.dragged` wird nur beim Druck zurueckgesetzt, nicht beim Loslassen.
Touch entkommt dem, weil sein Slot beim Loslassen ganz genullt wird; die
Maus behaelt ihren Slot. Danach traegt jede Hover-Bewegung `Dragged`, der
Scroller haelt das fuer einen Zug, und der Rueckgabewert von
`StealPointer`, der genau das verhindern wuerde, wird verworfen.

Kein Test hat das gefunden, weil `Swipe` und `Fling` im Harness den
Touch-Pfad benutzen und kein Test die Maus **nach** dem Loslassen bewegt.
Der Regressionstest muss beides tun.

Dazu ein sichtbarer und greifbarer Scrollbalken fuer `ScrollView` und
`ImageGallery`. Die Geometrie liegt in `ScrollInfo` vollstaendig vor. Der
Griff muss `EventPointerMove` konsumieren, sonst nimmt ihm der Viewport
den Zug wieder ab.

Ergebnis: Galerie ohne klebendes Scrollen, mit Balken.

### Schritt 7: Fundamente

Abschnitt 18, Abschnitt 17, der Rune-Kanal aus Abschnitt 19 und
Abschnitt 20. Diese vier tragen alles Weitere und gehoeren deshalb vor die
Komponenten.

Ergebnis: scharfe Ausgabe auf 2x, eingebettete Schriften, stabile Goldens
fuer Anwender, ein Zeichen aus einer deutschen Tastatur kommt an, und ein
Beispiel schaltet zur Laufzeit zwischen hell und dunkel.

### Schritt 8: Eingabe und Icons

`ui.TextField` nach dem Muster von Button, Bildschirmtastatur, Icons.

Ergebnis: ein Eingabefeld mit Umlauten, Auswahl, Einfuegen und
Bildschirmtastatur im Kioskmodus.

### Schritt 9: Komponenten

Auswahl nach den Human Interface Guidelines: `ui.Toggle`, `ui.Slider`,
`ui.SegmentedControl` und `ui.ProgressBar`, jeweils nach dem Muster von
`ui.Button`. Navigation ueber eine TabBar, einen NavigationStack und eine
modale Ebene. Kitchen-Sink-Demo.

Ein inaktiver Tab und ein ueberdeckter Schirm bleiben **gemountet** und werden
nach Abschnitt 5 aus dem Bild genommen. Abmounten waere der Verlust von
Scrollposition und halb getippter Eingabe, was auf einem Kiosk der teuerste
Fehler ist. Gemountet **und gezeichnet** waere schlimmer, weil gift beim
Zeichnen nichts verwirft: eine unbestimmte Fortschrittsanzeige in einem
unsichtbaren Tab hielte das Geraet dauerhaft wach. Die modale Ebene ist die
ausdrueckliche Ausnahme — der Inhalt darunter wird weiter gezeichnet, sonst
waere der Abdunkler ein graues Rechteck.

Eingaben unter einer modalen Ebene faengt ein bildschirmfuellender Abdunkler,
die Tastatur eine Fokusfalle. Der Abdunkler allein genuegt nicht, weil
Tabulator keine Zeigerposition hat. `bildschirmfuellend` ist dabei das
tragende Wort und wird erzwungen: ein Navigationsbehaelter auf einer
unbegrenzten Achse wird abgewiesen, statt still zu schrumpfen. Ein Abdunkler
der Groesse null unter einem normal gezeichneten Dialog ist ein
Bestaetigungsdialog, der unbemerkt zur Dekoration wird, und das ist die
schlimmste Fehlerklasse dieses Projekts.

**Listenkomponenten.** `ui.List`, `ui.Row`, `ui.Card`, `ui.Divider` und
`ui.Badge`, dazu `cmd/example-kitchensink` als erster Schirm, der die ganze
zweite Lieferung gleichzeitig zeigt. `ui.List` **virtualisiert nicht**: jede
Zeile wird gebaut, gemessen und gezeichnet. Das ist eine Entscheidung und
keine fehlende Mechanik — `ui.Gallery` recycelt Kacheln und beweist, dass der
Mechanismus existiert —, und sie gilt nur unterhalb zweier gemessener
Grenzen, die in der Godoc der Komponente stehen muessen:

1. Ein Rebuild kostet linear etwa 2,8 µs je Zeile auf der Entwicklungs-
   maschine. Mit dem **angenommenen** Faktor zehn fuer den Pi 4 und einem
   halben Frame Budget sind das rund 280 Zeilen. Der Faktor ist eine Annahme
   und auf der Zielhardware nachzumessen.
2. Die Shaping-Grenze aus Abschnitt 11, in Zeilen ausgedrueckt rund 750.

Beide Zahlen gelten fuer den Ruhezustand ebenso wie fuer den Rebuild, und
beide werden derated angegeben oder gar nicht. Eine Kennzahl, die in einem
Dokument ohne Umrechnung steht, in dem alles andere umgerechnet ist, ist
schlimmer als keine — sie sieht um den Faktor zehn billiger aus, als sie ist.

**Die schmalste gepruefte Breite ist 360 logische Pixel**, bei den Dichten 1,
1,5 und 2. Eine Pruefung, die nur bis 480 reicht, prueft die Aussage nicht,
die das Beispiel ueber sich selbst macht.

Verbindlich fuer jedes dieser Steuerelemente:

- Die Trefferflaeche ist von der gezeichneten Groesse unabhaengig und misst
  mindestens 44 logische Pixel auf jeder Achse. Der Zielbildschirm ist ein
  Touchpanel; ein 31 Pixel hoher Schalter ist der richtige *Anblick* und ein
  schlechtes *Ziel*. Umgekehrt zeichnet ein Steuerelement nie ausserhalb seiner
  eigenen Grenzen: sichtbar, wo man nicht hinfassen kann, ist der gleiche
  Fehler andersherum.
- Ein ziehbarer Teil nimmt die Zeigererfassung und **konsumiert
  `EventPointerMove`**, sonst nimmt ihm ein umschliessender Viewport den Zug
  wieder ab — dieselbe Regel, die Schritt 6 fuer den Scrollbalken aufstellt.
- Der Ursprung einer Zieh-Geste wird **einmal beim Druck** festgehalten und nie
  aus dem aktuellen Wert neu berechnet. Ein Regler baut sich bei jeder
  Wertaenderung neu auf; die Inversion, die Review-Gate 7 im Scrollbalken
  gefunden hat, waere hier der Normalfall und kein Grenzfall.
- Eine Auswahl faellt an der **Position des Loslassens** und nicht an der des
  Drucks, und ein gezogenes Loslassen wird nicht abgelehnt. `DragSlop` sind
  8 logische Pixel, auf dem Zielpanel etwa 1,3 Millimeter: ein Finger, der
  beim Abheben abrollt, verliert die Auswahl sonst ganz. Umschliessende
  Scrollflaechen leiden nicht darunter, weil ein Scroller den Zeiger schon
  bei der Bewegung an sich nimmt und der Entzug das Steuerelement abbricht.
- Jede Animation hat eine Frist und endet. Die einzige Ausnahme ist die
  unbestimmte Fortschrittsanzeige, die sich aus ihrem eigenen Painter
  erneuert. Ihr Preis gehoert in die Dokumentation der Komponente, und zwar
  richtig formuliert: gift verwirft beim Zeichnen nichts, was ausserhalb des
  Sichtfelds liegt. Die Anzeige haelt das Geraet wach, solange sie
  **gemountet** ist — Wegscrollen aendert daran nichts, nur das Entfernen
  aus dem Baum.

### Schritt 10: Beispiele

Liquid Glass mit Animation, Persistenz mit Eingabefeld. HEIF nach
Abschnitt 22 ist unabhaengig und kann jederzeit vorgezogen werden.

## 24. Dateiauswahl, virtuelle Liste und Sheet

Angefordert nach der zweiten Lieferung: eine Dateiauswahl. Wie in
Abschnitt 14 wird das Wachstum hier festgehalten und begruendet, statt nur
im Code zu passieren.

Kein nativer Dialog. Ebitengine hat keinen, ein Kiosk im Vollbild hat oft
keinen Desktop, der einen liefern koennte, und ein nativer Dialog laesst
sich nicht headless testen. Die Auswahl besteht deshalb aus gift-Views und
liest ueber ein `fs.FS`, das die Anwendung uebergibt. Damit bestimmt die
Anwendung, was sichtbar ist, und ein Test reicht ein `fstest.MapFS` herein.
Geschrieben wird nie.

Drei Bausteine, in dieser Reihenfolge:

- `ui.VirtualList`. `ui.List` baut jede Zeile und hat die in seiner
  Dokumentation gemessene Grenze von etwa 280 Zeilen auf dem Pi 4. Ein
  Verzeichnis mit zehntausend Dateien braucht dasselbe, was die Galerie aus
  Abschnitt 10 fuer Bilder hat: einen Pool, der beim Layout an Eintraege
  gebunden wird. Zeilen sind deshalb Daten (`ui.ListRow`) und keine Views,
  denn ein View wird nur durch einen Build zu einem Knoten. Gemessen: ein
  Scroll-Frame kostet 2,3 µs und alloziert nichts, bei tausend wie bei
  hunderttausend Eintraegen. Dafuer kam `LayoutContext.SetChildLabel` in den
  Kern, damit ein recycelter Knoten den Namen des Eintrags traegt, den er
  gerade zeigt.
- `ui.Sheet`, aus den Teilen von `ui.Modal`: Scrim, Fokusfalle, Ebene, dazu
  ein Panel, das von der Unterkante hereinfaehrt. Escape kommt ueber
  `KeyFallback` an, auch wenn auf dem Touchscreen nichts fokussiert ist.
  Ziehen zum Schliessen gibt es nicht.
- `ui/filepicker` mit den Modi Open, OpenMulti, Directory und Save. Ein
  Verzeichnis wird ausserhalb der UI-Goroutine gelesen. Dafuer kam
  `Token.Post` in den Kern, das Muster aus Abschnitt 5 in einem Aufruf. Eine
  Liste, die ankommt, nachdem die Person weitergegangen ist, wird verworfen.

Bewusst nicht enthalten: Vorschaubilder in der Dateiliste, eine Liste von
Orten wie Laufwerken, das Beobachten eines Verzeichnisses und eine
Auswahl ueber mehrere Verzeichnisse hinweg.

## 25. Video: ein integrierter Player

Stand 2026-10-30. Die ersten Unterabschnitte sind die Vorarbeit mit dem
Prototyp `cmd/gift-videoproto`; "Umsetzung in gift" am Ende beschreibt, was
daraus in `render`, `backend/ebiten`, `video` und `ui/videoplayer` geworden
ist. Angefordert fuer ein
Folgeprojekt: ein Videoplayer mit Ton, dessen API an AVPlayer und SwiftUIs
`VideoPlayer` angelehnt ist, ohne cgo, mit dem Raspberry Pi 5 unter
Raspberry Pi OS als erstem Ziel. Erwartet werden H.264 und H.265 in PAL
(4:3 und anamorph 16:9), HD und Full HD bei 25, 30, 50 und 60 fps.

Die Machbarkeitspruefung baute und mass:

- `video/ffmpeg`: Demuxer, Video- und Audiodecoder ueber das FFmpeg des
  Systems, per purego geladen.
- `video`: ein Player mit A/V-Clock, Seek und Ton als `io.Reader`.
- `cmd/gift-videoproto`: Ebitengine direkt, ohne gift-Views, mit
  YUV-Shadern und Messausgabe.

### Entscheidung: das FFmpeg des Systems, per purego

Das Muster ist dasselbe wie bei `asset/turbojpeg` (Abschnitt 22):
dlopen ueber purego beim ersten Gebrauch, ohne Bibliothek meldet der Player
`ErrUnavailable`, und nichts sonst bricht. Gebunden werden libavformat,
libavcodec, libavutil und libswresample. Raspberry Pi OS bringt alle vier
mit, auf dem Mac liefert sie Homebrew (`ffmpeg`).

Verworfen:

- **Codec in reinem Go.** Es gibt keinen brauchbaren H.264- oder
  HEVC-Decoder in Go, und keiner kaeme auf dem Pi an libavcodec mit NEON und
  Frame-Threading heran.
- **WebAssembly-Decoder (wazero).** Kein SIMD auf ARM in der Groessenordnung
  von NEON. Fuer 1080p60 aussichtslos.
- **GStreamer oder mpv.** Beide bringen eigene Ausgabepfade mit, die nicht
  in eine Ebitengine-Textur zeichnen, und eine eigene Thread- und
  Callback-Welt. libmpv mit Render-API braucht einen GL-Kontext, den
  Ebitengine nicht herausgibt.
- **V4L2 direkt.** Waere fuer HEVC moeglich (siehe unten), loest aber weder
  Demuxing noch H.264 noch Ton, und das FFmpeg von Raspberry Pi OS kapselt
  genau diesen Weg bereits.

**Der Preis ist die ABI.** FFmpegs API besteht aus Structs, nicht aus
Accessoren, und deren Offsets wandern zwischen Major-Versionen. Gelesen
werden einige Dutzend Felder (`video/ffmpeg/abi.go`). Fuer jede
Kombination der Majors von libavutil, libavcodec und libavformat gibt es
eine Tabelle, die `testdata/offsets.c` aus den Headern der jeweiligen
Version erzeugt, nie von Hand. Vorhanden sind FFmpeg 5.1 (Bookworm), 6.1
(Ubuntu 24.04), 7.1 (Trixie) und 8.0 (Homebrew). Innerhalb einer Major
haengt FFmpeg nur an, eine Tabelle deckt also alle Minor-Releases ab. Die
Tabelle fuer 7.1 aus den Originalheadern ist identisch mit der, die der Pi
aus den Headern seines rpt-Forks erzeugt; das bestaetigt das Verfahren.
Pixelformate werden zur Laufzeit ueber `av_get_pix_fmt` beim Namen
aufgeloest, weil das Enum zwischen Majors waechst. Callbacks aus FFmpegs
Decoder-Threads nach Go gibt es nicht: `get_format` bleibt der
Standard von libavcodec, der das Hardwareformat von selbst waehlt, sobald
ein Geraet angehaengt ist.

Windows ist wie bei turbojpeg nicht vorgesehen. Getestet ist nur
Linux/arm64 auf dem Pi. Fuer macOS ist der Code geschrieben und
kompiliert (`GOOS=darwin go vet`), lief aber mangels Go und FFmpeg auf dem
Entwicklungsrechner noch nicht.

### Decoder auf dem Pi 5

Der Pi 5 hat **keinen H.264-Decoder** mehr in Hardware, nur noch einen
HEVC-Block (`rpi-hevc-dec`, V4L2 stateless). Das FFmpeg von Raspberry Pi OS
erreicht ihn ueber den Hwaccel `drm` (v4l2-request). H.264 laeuft also immer
in Software. Gemessen auf einem Pi 500 (BCM2712, 4x A76 bei 2,4 GHz,
Trixie, FFmpeg 7.1.5+rpt2), Decoder inklusive Kopie in gifts
Upload-Layout, ohne Ausgabe, Testmaterial siehe unten:

| Datei | Decoder | Bilder/s | Echtzeit |
| --- | --- | --- | --- |
| H.264 1080p60 | Software, 4 Threads | 222 | 3,7x |
| HEVC 1080p60 | Software, 4 Threads | 224 | 3,7x |
| HEVC 1080p60 | Hardware | 342 | 5,7x |
| HEVC 1080p60 10 Bit | Software | 99 | 1,65x |
| HEVC 1080p60 10 Bit | Hardware | 163 | 2,7x |
| H.264 PAL 50i | Software | 848 | 34x |

Software genuegt fuer alle 8-Bit-Faelle der Anforderung. Der
Hardwaredecoder halbiert die CPU-Last bei HEVC und ist fuer 10 Bit noetig.
Er wird mit `Options.HW` angefordert und faellt still auf Software zurueck.

### Ausgabe: YUV bis auf die GPU

Ein Bild bleibt YUV, bis der Shader es umrechnet. Eine CPU-Wandlung nach
RGBA kostete pro Full-HD-Bild mehrere Millisekunden und schickte 8 statt
3 MB je Bild zur GPU. Der Decoder packt Y und die verzahnten
Chroma-Ebenen in **ein** Byte-Slice, das als **eine** RGBA-Textur von
Stride/4 x Hoehe*1,5 mit einem `WritePixels` hochgeladen wird.
Ebitengine kennt nur RGBA8, also nimmt der Kage-Shader die Bytes wieder
auseinander. Zwei Layouts: NV12 fuer 8 Bit, P016 (16 Bit, U und V verzahnt)
fuer 10 Bit. Planares Chroma (yuv420p) wird beim Kopieren verzahnt, eine
Schleife ueber ein Viertel des Bildes.

Drei Erkenntnisse aus der Messung, alle am Pi 5 bei 1920x1080 und 60 Hz:

1. **Verzweigungen und Fetches kosten, nicht die Arithmetik.** Die erste
   Shader-Fassung waehlte das Byte im Texel ueber eine `if`-Kette und holte
   U und V getrennt: rund 11 ms je Full-HD-Konvertierung. Mit
   verzweigungsfreier Auswahl ueber eine Maske und verzahntem Chroma (ein
   Fetch liefert U und V) sind es 6,1 ms mit Nearest-Chroma und 8,7 ms
   bilinear. Gemessen ohne VSync als Differenz zu einem Lauf, der nur das
   letzte Bild blittet (11,3 ms je Frame, Compositor eingeschlossen).
2. **Jeder Vollbild-Durchgang zaehlt** (wie bei `Config.DirectToScreen`
   bereits fuer den Pi 400 festgehalten). Der Weg ueber eine
   RGB-Zwischentextur und Ebitengines Offscreen kostet drei Durchgaenge und
   schaffte nur 43 fps. Konvertieren und Skalieren in **einem** Durchgang
   direkt auf den Framebuffer (`DrawFinalScreen`) erreicht mit VSync
   59,4 fps. Die Reserve ist klein: ohne VSync 15,9 ms je Frame mit
   bilinearem Chroma, 13,1 ms mit Nearest-Chroma.
3. **Beim Hochskalieren dreht sich das um.** Der fusionierte Shader muesste
   Luma selbst bilinear interpolieren, vier Fetches je Pixel des grossen
   Bildes. PAL und HD auf Full HD erreichten so nur 54 bis 57 fps. Hier
   gewinnt der Zwei-Pass-Weg: Konvertierung in Quellaufloesung, dann ein
   Blit mit dem Filter der Textureinheit, ebenfalls direkt auf den
   Framebuffer.

Daraus folgt die Regel des Prototyps (`-mode auto`): Bei Massstab 1 und
quadratischen Pixeln wird fusioniert, sonst zweistufig. Sie gehoert in den
Renderer, nicht in die Anwendung.

Der Upload selbst ist unkritisch: 1,1 bis 1,9 ms CPU je Full-HD-Bild, und
ein Lauf, der jedes Bild hochlaedt und nur blittet, schafft ohne VSync
83,8 statt 88,3 Bilder/s. Das `glFinish` vor `TexSubImage2D`
(Abschnitt 11) faellt also nicht ins Gewicht.

### Ton und Clock

Ton laeuft ueber `ebiten/v2/audio`, darunter oto v3.5: PulseAudio in reinem
Go (auf dem Pi pipewire-pulse), ALSA als Rueckfall per purego. Kein cgo.
Ein Prozess hat genau einen Audio-Kontext mit fester Rate. Der Decoder
resampelt deshalb mit libswresample auf diese Rate (48 kHz) und Stereo
float32. Der Player liefert den Ton als `io.Reader`
(`AudioStream`), der nie blockiert: Fehlt Ton, gibt er Stille aus, und die
Clock haelt an.

Master ist das Audiogeraet. Dessen Position springt aber in den Schritten,
in denen das Geraet Daten holt, und ein Bild, das nach einer springenden
Clock gewaehlt wird, wird bei 60 Hz abwechselnd doppelt gezeigt und
uebersprungen. Die Clock ist deshalb eine Wanduhr, die mit einem
Sechzehntel des Fehlers je Bild an die Audioposition gezogen wird und ab
60 ms Abweichung springt (`video/clock.go`). Gemessen: Bild zu Clock
innerhalb eines Bildes (+-17 ms), Korrekturen unter 10 ms, keine
Audio-Unterlaeufe.

Nach einem Seek gibt der Player einen neuen `AudioStream` aus, weil der
Ton, den das Geraet schon gepuffert hat, zur alten Position gehoert und
sich nur mit dem Geraete-Player verwerfen laesst. Seeks sind bildgenau:
Die Decoder beginnen am Keyframe davor und verwerfen alles vor dem Ziel,
auch innerhalb eines Audio-Frames.

Eine Sperre ist zu beachten: Ebitengine haelt seinen Mutex, waehrend es
`Read` aufruft. Die Position des Geraete-Players darf deshalb nie unter dem
Mutex des Players gelesen werden.

### Messungen der Wiedergabe

Pi 500, Trixie, labwc mit XWayland, 1920x1080 bei 60 Hz, Vollbild, VSync,
`-mode auto`, je 8 bis 10 s. Material: Big Buck Bunny (Blender
Foundation, CC BY 3.0), 1080p60, daraus 30 s ab Minute 1 umkodiert
(x264 `fast` CRF 21, x265 `ultrafast` CRF 24, AAC 160 kbit/s).

| Datei | Decoder | Pfad | Draws/s | spaet | CPU (1 Kern = 100 %) |
| --- | --- | --- | --- | --- | --- |
| PAL 16:9 anamorph H.264 25p | SW | zweistufig | 59,8 | 0 | 17 % |
| PAL 16:9 H.264 50i, Bob | SW | zweistufig | 59,8 | 0 | 18 % |
| HD H.264 50p | SW | zweistufig | 59,8 | 1 | 43 % |
| HD H.264 60p | SW | zweistufig | 59,8 | 2 | 48 % |
| Full HD H.264 60p | SW | fusioniert | 59,4 | 19 | 81 % |
| Full HD HEVC 60p | SW | fusioniert | 59,5 | 2 | 83 % |
| Full HD HEVC 60p | HW | fusioniert | 59,7 | 4 | 45 % |
| Full HD HEVC 60p 10 Bit | HW | fusioniert | 57,6 | 18 | 82 % |
| Full HD HEVC 60p 10 Bit | SW | fusioniert | 48,7 | 102 | 204 % |

„spaet“ zaehlt Bilder, die verworfen wurden, weil schon das naechste
faellig war. Seitenverhaeltnisse (PAL 4:3 mit Pillarbox, 16:9 anamorph
entzerrt) und Farben (BT.601 fuer SD, BT.709 fuer HD, begrenzter
Wertebereich) wurden per Screenshot geprueft, nicht messtechnisch.

Zwei Fallen, die beim Messen auffielen:

- `swayidle` schaltet den Ausgang nach 600 s ohne Eingabe per `wlopm`
  ab. Danach liefert XWayland keine Frame-Callbacks, und jeder Swap mit
  VSync wartet eine Sekunde. **Ein Videoplayer muss den Bildschirmschoner
  waehrend der Wiedergabe sperren** (AVPlayers
  `preventsDisplaySleepDuringVideoPlayback`). Wie das unter labwc aus einem
  XWayland-Fenster geht, ist offen, siehe unten.
- Auf dem Framebuffer des OpenGL-Backends laeuft `dstPos` von unten nach
  oben. Der fusionierte Shader rechnet seine Position deshalb aus den
  Quellkoordinaten der Vertices.

### API

AVFoundations Vokabular, gifts Form. Der Player ist ein Objekt mit
Lebensdauer und gehoert deshalb in den State der Anwendung, nicht in einen
View; der View ist ein Fenster auf ihn.

```go
p, err := video.Open(path, video.Options{HW: true})    // AVPlayer(url:)
p.Play(); p.Pause(); p.Seek(90 * time.Second)          // seek(to:) bildgenau
p.CurrentTime(); p.Duration()                          // currentTime(), duration
p.Status()            // StatusUnknown, StatusReadyToPlay, StatusFailed
p.TimeControlStatus() // Paused, WaitingToPlay, Playing
p.SetVolume(0.8); p.SetMuted(true); p.SetLoop(true)    // AVPlayerLooper light
p.AudioTracks(); p.SelectAudioTrack(1)                 // select(_:in:), audible
p.OnEnd(func() { ... })                                // DidPlayToEndTime

videoplayer.New(p).                    // SwiftUI VideoPlayer(player:)
	Gravity(videoplayer.ResizeAspect). // AVLayerVideoGravity: Aspect, AspectFill, Resize
	CornerRadius(12).
	AutoHide(3 * time.Second)
videoplayer.Surface(p).OnTap(f)        // nur das Bild, fuer eigene Controls
```

Die Komponente liegt in einem Unterpaket von `ui`, wie `ui/filepicker`:
`ui` bleibt ohne Abhaengigkeit auf `video` und damit auf das FFmpeg-Binding.

Bewusst nicht umgesetzt:

- `AddPeriodicTimeObserver`. Die Controls bauen sich ueber einen Token-Tick
  neu auf, viermal je Sekunde, solange sie sichtbar sind und das Video
  laeuft; die Positionsleiste malt sich jeden Frame aus dem Player, ohne
  Build (Abschnitt 11).
- Eine Quelle ueber `asset.Source` statt eines Pfades. libavformat liest
  ueber `AVIOContext` auch aus Go, das braeuchte aber Callbacks aus
  FFmpeg-Threads nach Go, die bisher vermieden sind. Pfade und `file:`-URLs.

### Integration in render und backend/ebiten

- **Ein neuer Ressourcentyp statt `render.Images`.** Ein Bild ist dort
  unveraenderlich, budgetiert und verdraengbar; ein Video ist eine
  Textur, die jeden Frame neu beschrieben wird. Vorschlag:
  `render.Videos` mit `Acquire(w, h, layout)`, `Update(handle, Pixels)` und
  einer eigenen Op oder einem `ImageFormat`, das die YUV-Materialien
  waehlt. Ein Video-Upload je gezeichnetem Frame wird nicht gegen das
  Thumbnail-Budget gerechnet.
- **Zwei Materialien in `backend/ebiten`**, die beiden Shader des
  Prototyps. Die Pfadwahl aus Massstab und Pixelseitenverhaeltnis trifft
  der Renderer. Abgerundete Ecken bekommt das Video wie
  `roundimage.kage` ueber die Abdeckung derselben Distanzfunktion.
- **Fusioniert nur mit `DirectToScreen`.** Ohne liegt immer Ebitengines
  Offscreen dazwischen. Ein Video, das nicht die oberste Ebene ist (ein
  Overlay darueber), braucht ohnehin die Reihenfolge der Display-Liste;
  fusioniert heisst dann nur: kein Zwischenziel fuer die RGB-Wandlung.
- **DrawOnDemand.** Ein spielender Player haelt seinen Knoten ueber
  `Animate` fuer Repaints offen, ein pausierter nicht.
- **Interlaced** wird im Prototyp mit Bob auf 50 Halbbilder je Sekunde
  gezeigt, das Halbbild waehlt die Clock. Fuer bessere Qualitaet waere
  bwdif aus libavfilter die naechste Stufe (weitere Bindung).

### Offen und Risiken

- **10 Bit in Full HD 60p** ist mit Hardwaredecoder knapp (57,6 fps), in
  Software nicht machbar. Der Transfer aus dem SAND-Format des Decoders
  kostet CPU. Naechste Versuche: Transfer in ein 8-Bit-Format, falls das
  rpt-FFmpeg ihn anbietet, oder DRM-PRIME-Frames als dmabuf ohne Kopie, was
  aber an Ebitengine vorbei auf GL-Ebene ginge.
- **Bildschirmschoner.** swayidle wertet das Wayland-Protokoll
  idle-inhibit aus, das ein XWayland-Fenster nicht sprechen kann.
  Kandidaten: `org.freedesktop.ScreenSaver.Inhibit` ueber D-Bus in reinem
  Go, falls labwc oder ein Dienst es auswertet, oder das Kiosk-Image ohne
  swayidle. Muss auf dem Zielimage geklaert werden; auch der umgesetzte
  Player sperrt den Bildschirmschoner noch nicht.
- **Kadenz.** 25p auf 60 Hz zeigt Bilder abwechselnd zwei- und dreimal.
  Das ist systembedingt; ein Kiosk mit PAL-Material sollte den Ausgang auf
  50 Hz stellen. Bei 60p auf 60 Hz verwirft H.264 in Software einige
  Bilder je 10 s (siehe Tabelle); mit Nearest-Chroma sind es weniger
  (13 statt 19 in 8 s). Die Bildwahl koennte sich ausserdem an die
  gemessene VSync-Phase haengen.
- **ABI-Pflege.** Jede neue FFmpeg-Major braucht eine Tabelle. Ohne
  Tabelle meldet das Paket `ErrUnavailable`, es liest nie mit falschen
  Offsets.
- **Recht.** H.264 und H.265 sind patentbelastet, wie HEIF (Abschnitt 22).
  gift liefert keinen Codec aus, decodiert wird mit dem FFmpeg des Systems.
  Ob und welche Lizenzen der Betreiber eines Geraets braucht, klaert er
  selbst. Dies ist keine Rechtsberatung.
- **Nicht betrachtet:** Untertitel, mehrere Tonspuren, HDR, Streaming
  ueber Netz, Wiedergaberaten ungleich 1, Mehrkanal-Downmix-Qualitaet
  (libswresample mischt 5.1 auf Stereo).

### Umsetzung in gift

Der Prototyp ist in gift aufgegangen, mit den Entscheidungen von oben und
drei Korrekturen aus der Messung im echten Framework.

- **Vertrag.** `render.OpVideo` ist eine eigene Op-Art (Bounds, Color,
  CornerRadius und Fit wie bei `OpImage`, Image ist eine Video-Id). Den
  Dienst dazu stellt das Backend als `render.Videos` bereit
  (`Present`/`Resolve`/`Release`), den Ton als `render.AudioOutput`. Beide
  reicht `App.SetVideos` an `PaintContext.Videos` und `PaintContext.Audio`
  weiter, auf demselben Weg wie `Images`. Ein Video ist nicht Teil des
  Bildbudgets und wird nie verdraengt.
- **Backend.** `backend/ebiten/video.go` haelt je Video eine YUV- und eine
  RGB-Textur. Die Pfadwahl aus "Ausgabe" trifft `Renderer.appendVideo` je
  Operation: direkt (ein Bildpixel je Geraetepixel, eckig, ohne Tint,
  progressiv) mit `videodirect.kage`, sonst einmal je neuem Bild nach RGB
  und weiter wie ein Bild, mit Filter, Eckenradius und Tint. Gecachte
  Ebenen vergleichen ein Video ueber einen Stempel, der sich mit jedem
  Bild aendert.
- **Ton.** `audioOutput` oeffnet Ebitengines Audio-Kontext beim ersten
  Strom und nimmt einen vorhandenen; `Config.AudioSampleRate` ist 48 kHz.
  Der Player verwaltet die Geraete-Player selbst (`AttachAudio`) und
  resampelt, falls das Geraet eine andere Rate hat. Ohne Geraet laeuft
  die Clock auf der Wanduhr und der Ton wird im Takt verworfen; das ist
  der Headless-Fall von `gifttest`.
- **Tonspuren.** `ffmpeg.File` listet alle Audiostroeme mit Sprache, Titel
  und Kanaelen (`AVStream.metadata` kam in die ABI-Tabellen) und wechselt
  per `SetAudioTrack`; der Player macht daraus `SelectAudioTrack`, einen
  Seek an die aktuelle Position mit dem Decoder der neuen Spur. Die
  ABI-Tabellen umfassen jetzt auch FFmpeg 9.0 (Homebrew).
- **Komponente.** `ui/videoplayer` bringt `Surface` (nur das Bild) und
  `New` (Bild mit Controls im Stil von iOS): Play/Pause zwischen
  Spruengen von 10 s, Positionsleiste mit verstrichener und
  verbleibender Zeit, Lautstaerke mit Stummschalter, Tonspurmenue.
  Kleinstes Touch-Ziel 44 Pixel, passend fuer 800x480. Die Leiste ist
  ein eigenes Control und kein `ui.Slider`, weil sie das Ende eines Zugs
  kennen muss: waehrend des Ziehens wird viermal je Sekunde gesucht,
  exakt erst beim Loslassen.

Die drei Korrekturen, alle am Pi 5 gemessen, Vollbild 1920x1080:

1. **Kein Hintergrund unter dem Bild.** Mit dem schwarzen Hintergrund der
   Flaeche und dem von `ui.Window` lagen zwei Vollbildfuellungen unter dem
   Video; FullHD 60p schaffte so 49 fps. Die Flaeche fuellt jetzt nur die
   Balken neben dem Bild, und das Beispiel nutzt kein `ui.Window`: 60,0 fps.
2. **Kein Scrim unter den Controls.** Eine durchscheinende Abdunklung des
   ganzen Bildes kostete bei sichtbaren Controls 1 bis 2 fps (58,4 statt
   60). Die Pillen hinter den Controls geben genug Kontrast.
3. **`DirectToScreen` ist Pflicht** fuer FullHD 60p, sonst liegt Ebitengines
   Offscreen dazwischen.

Gemessen mit `cmd/example-video -fullscreen -stats`, je 15 bis 20 s, Werte
nach dem Start:

| Datei | Decoder | Pfad | Controls | Frames/s |
| --- | --- | --- | --- | --- |
| Full HD H.264 60p | SW | direkt | aus | 60,0 |
| Full HD H.264 60p | SW | direkt | sichtbar | 60,0 |
| Full HD HEVC 60p | SW | direkt | sichtbar | 60,0 |
| Full HD HEVC 60p | HW | direkt | sichtbar | 60,0 |
| HD H.264 50p / 60p | SW | konvertiert | sichtbar | 60,0 |
| PAL 16:9 H.264 25p | SW | konvertiert | sichtbar | 60,0 |

Auf dem Mac (FFmpeg 9.0 aus Homebrew, Metal) laufen Binding, Player und
Komponente ebenso; Tests: `video`, `video/ffmpeg` und `ui/videoplayer`
mit einem Testclip von drei Sekunden und zwei Tonspuren
(`video/testdata/bbb.mp4`), `backend/ebiten/video_test.go` fuer die
Pfadwahl. Sie werden uebersprungen, wo FFmpeg fehlt.

### Offen

1. Bildschirmschoner waehrend der Wiedergabe sperren, auf dem Zielimage
   verifiziert.
2. 10 Bit in Full HD 60p (Hardware: 57,6 fps im Prototyp).
3. Untertitel, Wiedergaberaten ungleich 1, Quellen ueber `asset.Source`.
4. GPU-Golden-Tests je Layout und Matrix der beiden Video-Shader.

## 26. Bildbetrachter, Gesten und Multitouch

Angefordert fuer das Folgeprojekt: eine zoombare Bildkomponente, die sich
nahtlos mit der Galerie und dem Asset-Store verwenden laesst, eine
Vollbildvorschau mit Uebergang aus der getippten Kachel, Pinch-Zoom mit
korrektem Verhalten an den Grenzen, horizontales Blaettern, eine vertikale
Wischgeste fuer Karten, Desktop-Bedienung (Mausrad, einblendende Pfeile,
Tastatur), ein zweistufiges Laden fuer die Reaktionszeit auf dem Pi und die
Gestenlogik als eigene, wiederverwendbare Komponenten. Spaeter kommen weitere
Darstellungen hinzu, etwa ein Filmstreifen wie in Lightroom.

### Pakete

| Paket            | Inhalt                                                                         |
|------------------|--------------------------------------------------------------------------------|
| `ui/motion`      | Rubberband, Abbremsen, Projektion, Federn, Glide: geschlossene Formeln der Zeit |
| `ui/gesture`     | Tap, LongPress, Drag, Magnify, Rotate, Hover, Wheel; `On` und `Handle`         |
| `ui/imageviewer` | `Zoomable`, `Viewer` + `Fullscreen`, `Origin`; Zoom-, Pager- und Kartenphysik  |

Dazu im Kern: `Element.MultiTouch`, `MaxTouches`, `EventContext.Retained[T]`
(ein Wert pro Typ und Knoten, die offene Schwester von `ControlState`),
`EventContext.After` und `LayoutContext.After` mit `EventTimer` (Doppeltipp
und Long-Press brauchen Zeitentscheidungen ohne Ereignis) sowie
`App.PointerDownAt`/`MoveAt`/`UpAt` mit Messzeitpunkten. In `ui`:
`ui.Picture` als exportierter Zugang zum gemeinsamen Bilddienst,
`Gallery.ThumbnailFrame`, `Gallery.Conceal`, `GalleryView.OnOpen` und
`Symbol.Draw`. In `asset`: `DiskCacheConfig.MaxSize`. Im Backend:
`render.Pixels.Key` fuer bandweise Uploads, die Texturgrenze aus
`MaxImageSize`, `Config.Textures`, `Config.Touchscreen` und der evdev-Leser.

Die Filmstreifen-Darstellung ist nicht gebaut, aber vorbereitet: `Viewer`
traegt Katalog, Position und Zoom, `Fullscreen` ist nur eine Ansicht davon,
und `Origin` ist eine Schnittstelle, die ein Filmstreifen ebenso erfuellen
kann wie die Galerie.

### iOS als Vorbild: Konstanten und Quellen

| Groesse                       | Wert                                   | Quelle |
|-------------------------------|----------------------------------------|--------|
| Rubberband                    | (1 − 1/(x·0.55/d + 1))·d               | Grant Paul (iOS 6), gist.github.com/originell/6961057 |
| Abbremsen                     | 0.998 / 0.99 pro ms                    | UIScrollView.DecelerationRate, gemessen |
| Ruhepunkt eines Wurfs         | v/1000 · r/(1 − r)                     | WWDC 2018 "Designing Fluid Interfaces" |
| Schwelle einer Geste          | 10 pt                                  | ebenda; SwiftUI `DragGesture(minimumDistance: 10)` |
| Zurueckfedern                 | kritisch gedaempft, ca. 10 rad/s       | ScrollMechanics (k = 100); natives Setzen ~0.63 s laut Flutter-Issue 181752 |
| Zoom ueber die Grenzen        | Rubberband auf log(Zoom)               | iOS dokumentiert nur "animates just past these limits"; Herleitung hier |
| Anker beim Pinch              | p = (offset0 + c0)/s0, offset = p·s − c | Standardherleitung, Verhalten beobachtet |
| Doppeltipp                    | auf Fuellen, mindestens 2×, um den Tipp | iOS 18 fuellt; MWPhotoBrowser (min+max)/2 |
| Seitenabstand                 | 20 pt                                  | MWPhotoBrowser, Photos nicht verifiziert |
| Seitenwechsel                 | Wurf ueber 400 px/s, sonst halbe Seite | Flutter PageScrollPhysics |
| Wischen zum Schliessen        | Start bei ty > 0 und \|ty\| > \|tx\|, Fortschritt ty/(H/2), Schliessen bei 1200 pt/s oder 33 % | Apple-Beispiel PhotoTransitioning (2016) |

Die Erkenner laufen nebeneinander wie SwiftUIs simultaneous gestures, mit drei
Regeln: ein Tap wartet auf das Scheitern eines Taps hoeherer Zahl (UIKits
require-to-fail), ein ausgeloester Long-Press nimmt den Taps den Druck, und
ein Tap scheitert bei derselben Distanz, bei der ein Drag beginnt.
`ShouldBegin` entspricht `gestureRecognizerShouldBegin`. Ein Drag folgt wie
UIKits Pan-Erkenner dem Schwerpunkt aller Finger; die Translation springt
nicht, wenn ein Finger dazukommt oder geht.

### Zweistufiges, eigentlich dreistufiges Laden

1. Vorschau in `PreviewSize` (256 logische Pixel): die Groesse der
   Galeriekacheln, also beim Oeffnen aus der Galerie bereits im CPU-Cache und
   im ersten Frame des Uebergangs sichtbar. Sonst decodiert libjpeg-turbo bei
   1/8 aus den DC-Koeffizienten der 8×8-Bloecke — die "Makrobloecke". Die
   Entropie-Decodierung aller Koeffizienten bleibt bei Baseline-JPEG
   unvermeidlich; IDCT und Farbwandlung entfallen fuer 63 von 64 Werten.
2. Bildschirmgroesse.
3. Beim Zoomen eine groessere Stufe in Zweierpotenzen der Bildschirmgroesse,
   hoechstens `MaxDetail` (4096, die Texturgrenze, die jedes Ziel einhaelt).

Die Leiter des Pipelines braucht dafuer grosse Stufen; der Disk-Cache haelt
Rohpixel und darf sie nicht aufnehmen (`MaxSize`, Standard 1024).

### Touch auf dem Pi

Unter X11/XWayland kommt ein Touchscreen als emulierte Maus an: ein Finger,
keine Zeitstempel, eine Position pro Tick. Das Backend liest deshalb unter
Linux `/dev/input` direkt (Gruppe `input`, Protokoll B, `INPUT_PROP_DIRECT`),
cgo-frei ueber `x/sys/unix`, und ignoriert die emulierte Maus, solange ein
Finger liegt und bis der Cursor sich wirklich bewegt. Nachgebaut nach Android
und iOS:

- Kernel-Zeitstempel auf `CLOCK_MONOTONIC` (`EVIOCSCLOCKID`, direkt nach dem
  Oeffnen, sonst flusht der Kernel mit `SYN_DROPPED`), umgerechnet in die Uhr
  der Bruecke; `Event.Time` ist die Messzeit, Geschwindigkeiten rechnen damit
  (Android VelocityTracker: Ereigniszeiten, Horizont 100 ms, 40 ms Pause
  setzt zurueck).
- Resampling auf den Frame nach `InputConsumer` (libs/input/InputConsumer.cpp):
  Abtastzeit = Framezeit − 5 ms, Interpolation zwischen den umgebenden
  Proben, sonst Extrapolation aus den letzten zwei (2 ms ≤ Δ ≤ 20 ms,
  hoechstens 8 ms und Δ/2). Ein Test belegt den Effekt: 100-Hz-Proben bei
  60 Hz ergeben ohne Resampling Schritte von 20, 20, 10 px, mit Resampling
  gleichmaessig 16,7 px.
- Abweichung von Android: Androids 5 ms setzen Digitizer ab 120 Hz voraus.
  Die Treiber der Pi-Displays pollen alle 17 ms, mit HZ=250 real 20 ms. Fuer
  ein Panel langsamer als 12 ms folgt die Latenz der gemessenen Periode (+3 ms,
  hoechstens 30 ms), damit immer interpoliert wird.
- Ein Finger auf einer leerlaufenden Anwendung hebt die Tickrate sofort an.
- `cmd/gift-touchinject` erzeugt per uinput einen virtuellen
  Multitouch-Bildschirm. Damit sind Tipp, Wischen, Pinch und Doppeltipp auf
  dem Pi 500+ ohne Touchscreen ueber den echten Pfad getestet, einschliesslich
  der emulierten Maus des Compositors.

### Messungen auf dem Raspberry Pi 5

Full HD, 24 Bilder bis 12 Megapixel, libjpeg-turbo, Gestenfolge ueber den
virtuellen Touchscreen (Tipp, vier Seitenwechsel, Pinch auf, Pan, Pinch zu),
`giftmetrics`:

| Stand                                         | p50     | p95     | p99     | max     | Draw-CPU max |
|-----------------------------------------------|---------|---------|---------|---------|--------------|
| erste Fassung, 4 Worker                       | 16,7 ms | 18,2 ms | 26,0 ms | 90,5 ms | 40,8 ms      |
| bandweise Uploads                             | 16,8 ms | 18,2 ms | 23,5 ms | 57,4 ms | 4,4 ms       |
| + 2 Worker, DirectToScreen, Nachbarn im Stillstand | 16,6 ms | 17,4 ms | 20,1 ms | 54,2 ms | 1,9 ms |

Die Ursachen, einzeln protokolliert: ein Bild ueber dem Upload-Budget wurde
in einem Frame hochgeladen (11 MB Bildschirmstufe, 45 MB Detailstufe, 41 ms
Draw). Jetzt wird es in Zeilenbaendern ueber mehrere Frames hochgeladen
(`render.Pixels.Key`), ohne API-Aenderung, weil die Bruecke ohnehin weiterzeichnet,
solange ein Upload aussteht. Das Anlegen einer bildschirmgrossen Textur
kostet den V3D dennoch rund einen Frame, gleich wie klein das erste Band ist;
deshalb bereitet der Betrachter die Nachbarseiten nur vor, wenn nichts sich
bewegt. Vier Decoder auf vier Kernen verdraengen die Frameschleife (p99
24,2 statt 20,3 ms mit zwei).

### Offen

- Rotation ist erkannt, aber nicht im Betrachter verwendet.
- Kein Neuaufsetzen nach `SYN_DROPPED` ueber `EVIOCGMTSLOTS`; liegende Finger
  werden gehoben.
- Der Touchscreen wird dem Monitor des Fensters zugeordnet; gedrehte Displays
  und mehrere Monitore sind nicht behandelt.
- Tiefes Zoomen ueber 4096 Pixel braeuchte Kacheln (Tiles); `MaxDetail`
  begrenzt bis dahin.
- Ein Texturpool fuer gleich grosse Bilder koennte die Anlegekosten beim
  Blaettern durch eine Kamerarolle ganz vermeiden.
- Trackpad-Pinch auf dem Mac liefert GLFW nicht; dort zoomen Mausrad, Tasten
  und Doppelklick.
- Der Filmstreifen ist nicht gebaut.

## 27. Menues, Kontextmenues und Popover

Nach dem Abgleich mit iOS (Abschnitt 26 ff.) als erste fehlende Bausteine
gewaehlt, weil sie die Grundlage fuer Menue-Picker, Auswahllisten und spaetere
Popups sind.

### Die oberste Ebene im Kern

Ein Menue an einem Knopf tief im Baum kann kein gewoehnliches Kind seines
Knopfs sein: ein Scrollcontainer schneidet es ab, spaeter gezeichnete
Geschwister ueberdecken es, und ein Tipp daneben trifft, was darunter liegt.
`ui.Modal` loest das, indem es den Bildschirm umhuellt; fuer ein Steuerelement
ist das die falsche Form. Deshalb gibt es `Element.Overlay`: ein Knoten erklaert
eine Ansicht, der Kern haengt sie in eine oberste Ebene als zweites Kind der
Wurzel, vollflaechig, zuletzt gezeichnet und zuerst getroffen. Die Erklaerung
gehoert dem Knoten (SwiftUIs `.popover(isPresented:)`), wird mit ihm neu gebaut,
verschwindet sofort mit ihm oder wenn er verborgen wird, und die Ebene existiert
nur, solange etwas praesentiert wird. `LayoutContext.Presenter` liefert das
Rechteck des Ausloesers; bewegt er sich im laufenden Layout, wird die Ebene im
selben Update noch einmal gelegt. `Element.OverlayExit` laesst eine
zurueckgezogene Ebene ohne Eingabe nachlaufen, damit sie wegschrumpfen kann.

Dazu: `EventContextMenu` fuer die sekundaere Maustaste (sie tat bisher nichts),
`EventContext.Capture(id)` und ein `EventLongPress`, der zu den Vorfahren
durchgereicht wird und waehrenddessen `StealPointer` erlaubt. So sieht ein
Kontextmenue um einen Knopf den Long-Press, den der Knopf nicht nutzt, und nimmt
ihm den Finger, damit das Loslassen kein Tipp mehr ist. Niemand im Repository
hatte `EventLongPress` bisher verarbeitet.

### iOS als Vorbild

| Groesse              | Wert                                          |
|----------------------|-----------------------------------------------|
| Menuebreite          | 250 pt                                        |
| Zeilenhoehe          | 44 pt, Titel 17 pt, Symbol hinten 20 pt       |
| Abschnitte           | Haarlinie, 16 pt eingerueckt, 13 pt hoch (iOS 26), Ueberschrift 13 pt |
| Hervorhebung         | eingerueckt 6 pt, Radius 7 pt, konzentrisch zur Karte |
| Eckenradius          | 13 pt                                         |
| Pfeil                | 10 pt hoch, 20 pt breit                       |
| Abstand zum Rand     | 8 pt                                          |
| Long-Press           | 0,5 s (`LongPressDelay`)                      |
| Auswahl              | Druecken, Wischen, Loslassen ohne zweiten Tipp |

Das Menue erscheint beim Druck, unter oder ueber dem Knopf, buendig mit
dessen vorderer Kante, und waechst mit einer leicht ueberschwingenden Feder aus
der naechsten Ecke; das Popover waechst aus seiner Pfeilspitze. Ein
Kontextmenue nach Long-Press dunkelt das Fenster um den Inhalt ab; das Loch
folgt dem Eckenradius des Inhalts, gezeichnet als ein einziger, sehr breiter
abgerundeter Ring. Der Pfeil ist ein um 45 Grad gedrehtes Quadrat, dessen
innere Haelfte die Karte verdeckt; ein Dreieck-Primitiv gibt es nicht.

### Offen

- Kein Weichzeichnen hinter dem Kontextmenue wie bei iOS; nur Abdunkeln. Die
  Glass-Materialien koennten es, kosten aber auf dem Pi.
- Kein Hochheben (Skalieren) des Inhalts beim Long-Press.
- Keine Untermenues.
- Ausblenden ist ein Schrumpfen ohne Ueberblenden des Inhalts, weil gift keine
  Gruppen-Deckkraft hat.
- Ein Menue, das hoeher als das Fenster ist, wird abgeschnitten statt
  gescrollt.

## 28. Wheel-Picker, Datums- und Uhrzeitrad

Der zweite Baustein aus dem Abgleich mit iOS. Ein Rad ist auf einem
Touch-Kiosk der natuerlichste Picker: drehen, werfen, anhalten, nichts tippen.

- `ui.WheelPicker` mit beliebig vielen `ui.Wheel`-Spalten; die Zeilen liegen
  auf einem Zylinder (Lage R·sin θ, θ = d/R, R halbe Hoehe), werden nach aussen
  perspektivisch kleiner (0,7 + 0,3·cos θ) und blassen ueber zwei Verlaeufe in
  den Hintergrund aus; ein Band hinterlegt die Auswahl ueber alle Spalten.
- Nur vertikales Stauchen war die erste Fassung. Der Glyphen-Renderer rastert
  dann kleiner, behaelt aber die Laufweite, und die Zeilen erscheinen gesperrt;
  gleichmaessiges Skalieren hat das Problem nicht.
- Physik: Rubberband an den Enden (`motion.Band`), ein Wurf wird mit Apples
  Projektion (0,998/ms) auf eine Zeile gerundet und mit einer kritisch
  gedaempften Feder dorthin gebracht, also ohne Ueberschwingen; ein Finger auf
  einem drehenden Rad haelt es an; Tipp auf eine Zeile, Mausrad zeilenweise,
  Pfeile, Bild auf/ab, Pos1/Ende.
- `OnChange` meldet beim Loslassen die Zeile, auf der das Rad zur Ruhe kommt;
  eine von der Anwendung gesetzte Auswahl dreht das Rad auf kuerzestem Weg.
- `ui.DateWheel` (Tag, Monat, Jahr; Reihenfolge, Monatsnamen, Bereich,
  Monatslaengen) und `ui.TimeWheel` (Stunde, Minute, Minutenintervall) sind
  daraus gebaut. Zyklisch drehen Tag, Monat, Stunde und Minute.

Offen: 12-Stunden-Uhr mit AM/PM, Lokalisierung ueber die Monatsnamen hinaus,
eine Tonrueckmeldung beim Weiterrasten.

## 29. Kalender

Der grafische DatePicker von iOS, als Gegenstueck zu den Raedern.

- `ui.Calendar`: Kopfzeile mit Monat und Jahr, Pfeile fuer den vorigen und den
  naechsten Monat, Wochentagszeile, sechs Wochen in festen Zeilen von 44 pt,
  damit die Hoehe beim Blaettern nicht springt. Der gewaehlte Tag liegt in
  einem Kreis der Akzentfarbe, heute ist in der Akzentfarbe geschrieben, Tage
  ausserhalb des Bereichs sind grau und nehmen keinen Tipp an; `Marked` setzt
  einen Punkt unter Tage mit Terminen.
- Blaettern wie im Bildbetrachter: drei Monate liegen nebeneinander, ein Wisch
  verschiebt sie, ein Wurf ab 400 pt/s oder ein langsamer Zug ueber die Haelfte
  wechselt den Monat, eine kritisch gedaempfte Feder bringt ihn an; an den
  Enden des Bereichs gibt das Rubberband nach. Pfeile, Mausrad (hoechstens ein
  Monat je 200 ms, damit ein Trackpad nicht durch das Jahr rast) und Bild
  auf/ab schieben den neuen Monat von seiner Seite herein.
- Ein Tipp auf den Titel ersetzt Raster, Wochentage und Pfeile durch ein Monats-
  und ein Jahresrad (`ui.WheelPicker`); der Chevron dreht nach unten.
- Tastatur: Pfeile verschieben den gewaehlten Tag um einen Tag oder eine
  Woche, der angezeigte Monat folgt ihm.
- Der angezeigte Monat ist Zustand der Komponente, nicht der Anwendung: Blaettern
  waehlt nichts.

Offen: Wochennummern, Bereichsauswahl (von–bis), Lokalisierung der
Wochentage ueber `WeekdayNames` hinaus, eine Hoehe, die der Zahl der Wochen
folgt.

## 30. SecureField

Ein `TextField` mit Maske, als Gegenstueck zu SwiftUIs SecureField.

- `ui.SecureField(ed)` liefert ein `TextFieldView` mit gesetztem Schalter;
  Editor, Bindung, Callbacks, Aussehen, Fokus und Bildschirmtastatur bleiben
  dieselben. `Revealed` zeigt den Klartext (Augen-Knopf), `ShowsLastCharacter`
  das zuletzt getippte Zeichen fuer `SecureRevealDuration` (1,5 s) oder bis
  zur naechsten Eingabe; Standard ist der Schalter der Bildschirmtastatur,
  also an auf dem Kiosk und aus auf dem Desktop wie bei macOS.
- Gesetzt wird statt jedes Zeichens eine Ziffer „0“ als Zelle, gezeichnet wird
  statt ihres Glyphs ein Kreis von 0,38 em: der Punkt von U+2022 ist in den
  meisten Schriften klein und eng, und es gibt keine Laufweite im Textsatz.
  Die Ziffer hat in jeder Schrift etwa 0,55 em, das ist die Teilung von iOS.
  Caret, Auswahl und Klick laufen ueber eine Abbildung der Byte-Offsets
  zwischen Dokument und Maske.
- Kopieren und Ausschneiden tun nichts, Einfuegen geht (Passwortmanager).
  Keine Woerter: Doppelklick waehlt alles, Wortsprung geht an die Enden.
- Der gemeldete Wert (Barrierefreiheit, giftauto) ist die Maske, nie der
  Klartext.

Offen: das Leeren beim erneuten Bearbeiten, das UIKit fuer sichere Felder
macht, und ein Hinweis auf eingerastete Feststelltaste.

## 31. TextArea

Das mehrzeilige Textfeld, Punkt 5 der iOS-Liste. Der Name `TextEditor` ist
bei gift das Modell (Text, Caret, Auswahl), deshalb heisst die Ansicht
`ui.TextArea`; sie arbeitet auf demselben Modell wie `TextField`.

- Umbruch an der Breite, harte Umbrueche mit Enter; Einfuegen behaelt Zeilen,
  macht aus CR/CRLF/LS/PS ein `\n` und aus Tab vier Leerzeichen.
- Hoehe `Lines(min, max)`: waechst mit dem Text von min bis max Zeilen (0 =
  ohne Grenze), danach scrollt das Feld selbst (Rad, Finger, Caret im Blick,
  Ziehen ueber den Rand); ein schmaler Indikator rechts waehrend des Scrollens.
- Zeilengeometrie aus dem Absatz: `Line.Start/End` umfassen beim weichen
  Umbruch die Leerzeichen, beim harten liegt der Umbruch *zwischen* End und
  dem naechsten Start (der Kommentar im Textpaket behauptete das Gegenteil und
  ist korrigiert). Der Offset am weichen Umbruch ist zweideutig; eine
  Affinitaet (`up`) haelt ihn am Ende der oberen Zeile, wenn End oder ein
  Klick hinter das Zeilenende ihn dorthin gesetzt hat.
- Enter beginnt eine Zeile; mit der Shortcut-Taste ruft es `OnSubmit` auf,
  ohne `OnSubmit` bleibt es ein Zeilenumbruch.
- Tasten: hoch/runter mit gemerkter Spalte, Bild hoch/runter um die
  sichtbaren Zeilen, Pos1/Ende auf der Bildschirmzeile, mit der
  Shortcut-Taste an die Enden des Texts; Dreifachklick waehlt den Absatz.
- Zeiger: Die Maus waehlt beim Ziehen in jede Richtung aus; ein Finger, der
  senkrecht beginnt, scrollt (das Feld, sonst das Formular). Ein Finger setzt
  den Caret erst beim Loslassen, wie bei iOS, damit Scrollen ihn nicht
  versetzt. Die Unterscheidung nach Geraet ist moeglich, seit der Pi das
  Panel als Touch liest.

Offen: Undo, Schwung beim Scrollen mit dem Finger, Umbruch ueberlanger
Woerter, Auswahlgriffe fuer Touch.

## 32. Stepper, Suchfeld, Seitenpunkte, Toast

Punkt 6 der iOS-Liste: kleine Controls, die ein Kiosk staendig braucht.

- `ui.Stepper` wie UIStepper: 94 x 32 pt, Minus und Plus durch eine
  Haarlinie getrennt; ein Schritt schon beim Aufsetzen, gehalten nach 0,5 s
  zehnmal, nach 2 s zwanzigmal je Sekunde (`EventContext.After`; der
  Frametakt macht 50 ms zur feinsten Stufe). Am Ende des Bereichs wird die
  Haelfte grau, `Wraps` springt zum anderen Ende; Schritte liegen auf dem
  Raster ab dem Minimum, damit zehn Zehntel genau 1 ergeben.
- `ui.SearchField`: ein `TextField` in der Kapsel von UISearchBar (36 pt,
  Radius 10), Lupe und Loeschknopf selbst gezeichnet (die Symbolpakete
  importieren `ui`). Der Loeschknopf traegt `Element.PreservesFocus`, damit
  Fokus und Bildschirmtastatur bleiben; Escape leert, im leeren Feld geht
  Escape weiter nach aussen.
- `ui.PageControl` wie UIPageControl: Punkte 7 pt, Teilung 16 pt; Tipp neben
  den aktuellen Punkt blaettert, ein Finger entlang der Punkte scrubbt (der
  Gestenzustand ist `Retained`, weil jede Seite neu baut), senkrechte Zuege
  bleiben dem Container. `Capsule` legt die Punkte weiss auf eine dunkle
  Kapsel fuer Bilder.
- `ui.Toast`: Kapsel mit Symbol, Titel, Untertitel in der Top-Ebene, faellt
  von oben (oder unten) mit einer Feder herein, bleibt 2 s
  (`LayoutContext.After`), laesst sich zur Kante wegwischen, haelt unter dem
  Finger. Die Ebene ist fenstergross, aber ohne Interactor, damit ein Tipp
  neben der Kapsel durchgeht.

Offen: Stepper mit Wertanzeige wie SwiftUIs Stepper mit Label, eine
Abbrechen-Taste am Suchfeld, viele Seiten mit schrumpfenden Randpunkten, eine
Warteschlange fuer mehrere Toasts.

## 33. Wisch-Aktionen und Pull-to-Refresh

Punkt 7 der iOS-Liste: die beiden Gesten einer iOS-Liste.

- `ui.Swipe(row).Trailing(...).Leading(...)`: die Zeile laesst sich seitlich
  ziehen und deckt Aktionen auf (mind. 74 pt, Symbol ueber Titel, grau,
  destruktiv rot, eigene Toenung). Ueber die Haelfte oder mit Wurf bleibt sie
  offen, ein Tipp auf eine Aktion fuehrt sie aus und schliesst. Ganz
  durchgezogen waechst die erste Aktion ueber die Zeile und laeuft beim
  Loslassen. Die Richtung entscheidet die Summe der Bewegungen innerhalb der
  Schwelle, die von einem Knopf in der Zeile zu ihr aufsteigen; senkrecht
  bleibt der Liste. Geschwindigkeit verfaellt nach 50 ms Stillstand.
- Offen haengt die Zeile vier unsichtbare Faenger um ihre Aktionen in die
  Top-Ebene: ein Tipp anderswo schliesst nur, wie bei iOS. Der offene Zustand
  ist Komponentenzustand, damit Oeffnen und Schliessen die Faenger an- und
  abbauen.
- Pull-to-Refresh: im Kern `ScrollInfo.Overscroll` und
  `ScrollSpec.LeadingHold`, eine Ruhelage hinter dem oberen Ende, zu der die
  Feder des Gummibands statt zu 0 laeuft; ein Finger greift die Inhalte dort,
  wo sie sich zeigen (`rubberInverse`). `ScrollView.Refreshable` loest bei 70 pt
  noch waehrend des Ziehens aus und haelt 56 pt; der Spinner hat die acht
  Speichen von UIActivityIndicatorView, beim Ziehen erscheinen sie einzeln.

Offen: Pull-to-Refresh fuer `VirtualList`, das Auslaufen einer geloeschten
Zeile nach links wie bei iOS, Aktionen per Tastatur.

## 34. Boegen: Ring, Messanzeige, kreisfoermiger Slider

Punkt 8 der iOS-Liste. Die Display-Liste kannte nur Rechtecke, abgerundete
Rechtecke, Glyphen und Bilder.

- `render.OpArc`: der Bogen des Inkreises der Bounds, StrokeWidth breit, mit
  runden Enden, Start und Ueberstreichung im Uhrzeigersinn ab zwoelf Uhr.
  Die Winkel stehen als `MaterialArc` in der Material-Seitentabelle, damit
  `Op` bei 72 Byte bleibt.
- Im Backend ist der Bogen ein Fall desselben Shape-Shaders: ein negativer
  erster Slot markiert ihn (minus Radius der Mittellinie, halbe Breite,
  Start, Ueberstreichung, alles in Geraetepixeln), die Distanzfunktion ist
  Inigo Quilez' Bogen im Rahmen der Bogenmitte. Ein Quad, derselbe Draw-Call
  wie alles andere; GPU-Test `TestAnArcCoversItsSweepWithRoundEnds`.
- `ui.ProgressRing` (mit Federung zum neuen Wert, `Indeterminate` ist der
  Speichen-Spinner, jetzt mit Pull-to-Refresh geteilt), `ui.Gauge` (offen wie
  ein Tacho oder geschlossen als Kapazitaet, Beschriftung in der Mitte und an
  den Enden), `ui.CircularSlider` (Knopf des Sliders, Ziehen entlang des
  Rings, haelt an den Enden der Luecke, Tasten und Rad).

Offen: Farbverlauf entlang eines Bogens (Apples Gauge mit Gradient), Skalen-
striche, Wertmeldung fuer Anzeigen ohne Interactor.

## 35. Farbwaehler

Punkt 9 der iOS-Liste.

- Waagerechte Verlaeufe im Renderer: `GradientParams.Horizontal`,
  `render.HorizontalGradient`; die Eckfarben der Quads folgen x statt y, die
  GPU interpoliert wie bisher. GPU-Test von links nach rechts.
- `ui.ColorPicker` wie UIColorPickerViewController, 320 pt breit: Raster
  (Grautoene, zwoelf Farbtoene von dunkel bis blass), Spektrum (sechs
  waagerechte Verlaeufe zwischen Primaer- und Sekundaerfarben, darueber weiss
  nach oben und schwarz nach unten, neun Quads, exakt) und Regler fuer Rot,
  Gruen, Blau mit Spuren von Kanal 0 bis voll; Deckkraft ueber Schachbrett,
  Vorschau und Hex-Feld, das beim Tippen uebernimmt. Runde Ecken von Raster und
  Spektrum ueber eine Ringmaske in der Hintergrundfarbe, weil es keinen runden
  Clip gibt.
- `ui.ColorWell` wie das Feld von SwiftUIs ColorPicker: Farbe in einem Ring
  aus 24 Boegen aller Farbtoene, oeffnet den Waehler im Popover.
- `ui.HexColor` und `ui.ParseHexColor` fuer #RGB, #RRGGBB, #RRGGBBAA.

Offen: gespeicherte Farben und Pipette von iOS, Display-P3, Spektrum fuer
Farben, die weder volle Helligkeit noch volle Saettigung haben (die Lupe
steht dann naeherungsweise).

## 36. Neue Kitchen Sink, Showcase und Video mit Dateiauswahl

Die alte Kitchen Sink (vier Tabs, Stand der fruehen Arbeitspakete) ist durch
eine strukturierte ersetzt, die jede Komponente zeigt.

- Die Komponenten-Demos der Doku sind aus `docs/gallery` (package main) nach
  `internal/showcase` gezogen und eine Bibliothek: `All`, `Lookup`, `Groups`,
  `Samples`, je Demo `Title` und `Summary` aus den Doku-Seiten. Die Galerie,
  das Capture-Tool und die Kitchen Sink nutzen dieselben Demos; Hugo haengt
  das Verzeichnis weiter als `assets/gallery` ein. Ein Test baut und zeichnet
  jede Demo in ihrer Groesse.
- Kitchen Sink: Liste nach Abschnitten der Doku mit Suche, daneben die
  Komponente mit Titel, Satz und ihrer Demo auf einer Buehne in ihrer Groesse;
  unter 900 pt Breite wird die Liste zur Wurzel eines NavigationStack. Den
  Wechsel meldet der Layouter der beiden Fenster ueber einen Knoten-Timer
  (`LayoutContext.After`), weil ein Layout keinen State schreiben darf. Jede
  Demo ist eine eigene Komponente mit ihrem Namen als Schluessel. Schalter fuer
  Bildschirmtastatur und dunkles Design.
- Neu im Kern: `Context.OnUnmount(name, fn)`, laeuft beim Abbau einer
  Komponente vor dem Verwerfen ihres States; die Video-Demo schliesst damit
  ihren Player.
- Der Testclip liegt jetzt in `internal/sampleclip` und ist eingebettet
  (`sampleclip.Path`), damit Kitchen Sink, Galerie und Video-Beispiel ihn
  ueberall finden.
- Video-Beispiel: ohne `-file` die Dateiauswahl ueber dem Home-Verzeichnis
  (`-dir`), `-sample` fuer den eingebetteten Clip; `videoplayer.OnClose` setzt
  einen Schliessen-Knopf oben links, der zur Auswahl zurueckfuehrt.

## 37. Karten: eine Kartenansicht nach dem Vorbild von MapKit

Status: gebaut am 31.10.2026 in den Schritten M1 bis M7; was davon abweicht,
was gemessen wurde und was offen ist, steht in "Umsetzung und Messungen" am
Ende des Abschnitts.

Angefordert: eine eigene Kartendarstellung wie MapKit. Einsatz sind
klassische POI-Marker, vor allem aber Fotos mit Geokoordinaten wie in der
Karte der iOS-Fotos-App: beim Pinch-Zoom verschmelzen Fotocluster oder brechen
auseinander, was die Zahl der gezeichneten Dinge begrenzt und die Karte
lesbar haelt. Was ausserhalb des Sichtbaren liegt, wird nie gezeichnet. Als
zweites Szenario das Wetter als quasi dreidimensionale, parallaxe Ebenen
ueber der Karte: Wolken, Niederschlag, Wind.

### Datenquelle: OpenStreetMap ja, die Kachelserver der OSMF nur zum Entwickeln

OSM ist die richtige Datenbasis: weltweit, aktuell, frei unter ODbL. Pflicht
ist die sichtbare Namensnennung "© OpenStreetMap-Mitwirkende" auf der Karte,
ueblich unten rechts, nicht hinter einem Schalter und nicht unter
Bedienelementen.

Die Kachelserver der OSM Foundation (`tile.openstreetmap.org`) sind dagegen
keine Produktquelle. Ihre Richtlinie, gelesen am 31.10.2026, verlangt einen
eindeutigen, stabilen User-Agent (generische wie `Go-http-client/1.1` werden
blockiert), Caching nach den HTTP-Headern oder mindestens sieben Tage, und sie
verbietet Bulk-Download, Prefetch-Funktionen und Kachelarchive zur Weitergabe.
Es gibt kein SLA, der Zugang kann ohne Ankuendigung entzogen werden, und fuer
Offline-Karten verweist sie ausdruecklich auf Selbsthosten oder Anbieter, die
das erlauben. Fuer Entwicklung und Handtests passt das, fuer einen Kiosk auf
dem Pi, der auch ohne Netz laufen soll, nicht.

**Entschieden am 31.10.2026:** die Kartenquelle ist hinter einer Naht
abstrahiert, damit der Anbieter bei Bedarf wechseln kann (siehe "Die
Kartennaht"). Begonnen wird mit selbst gehosteten **Vektorkacheln aus OSM im
PMTiles-Format** (Protomaps), zuerst einem Ausschnitt von Oldenburg. Ein
PMTiles-Archiv ist eine einzige Datei, gelesen von der SD-Karte oder per
HTTP-Range-Request von einem beliebigen statischen Webserver, ohne
Kachelserver. Kommerzielle Anbieter (MapTiler, Stadia, Thunderforest) und die
OSMF-Server zum Entwickeln haengen an derselben Naht.

### Datenzugang, geprueft am 31.10.2026

| Quelle                                             | Befund                                                                                       |
|----------------------------------------------------|----------------------------------------------------------------------------------------------|
| OSM-Rasterkacheln `tile.openstreetmap.org`         | erreichbar; PNG 256², `ETag`, `max-age` etwa ein Tag, `stale-while-revalidate` sieben Tage   |
| Protomaps `build.protomaps.com/JJJJMMTT.pmtiles`   | erreichbar; taeglich, 139 GB, `Accept-Ranges: bytes`, Kopf `PMTiles` per Range gelesen      |
| Geofabrik-Extrakte                                 | erreichbar; Rohdaten je Region, nur noetig, falls eigene Kacheln gebaut werden               |
| DWD ICON-D2 (`clcl`, `clcm`, `clch`, `u_10m`, …)   | erreichbar; GRIB2, bz2, auch als regulaeres Lat/Lon-Gitter, nicht nur ikosaedrisch           |
| DWD Radar-Komposit `composite/rv`                  | erreichbar; alle fuenf Minuten ein tar-Archiv                                                |
| Open-Meteo                                         | erreichbar; JSON, global, kommerziell nur mit Abo                                            |
| RainViewer                                         | erreichbar; globale Radarkacheln, Bedingungen des freien Zugangs vor Nutzung pruefen         |

Tests laufen nie gegen das Netz, so wie die Galerie-Goldens ihre Bilder
erzeugen, statt sie einzuchecken: eine synthetische Kachelquelle, die z/x/y
und ein Gitter in jede Kachel zeichnet, ein Katalog von 100 000 Fotos mit
festem Seed in Haeufungen um Staedte, im Test erzeugte Vektorkacheln und
Wetterfelder.

**Der Oldenburg-Ausschnitt** fuer Handtests und Pi-Messungen liegt seit dem
31.10.2026 vor. Er ist nicht eingecheckt, weil ein Auszug der Datenbank unter
ODbL steht, sondern wird mit dem offiziellen Werkzeug von Protomaps geschnitten.
Das Werkzeug ist keine Abhaengigkeit von gift:

```sh
dir="$HOME/Library/Caches/gift/maps"   # Linux: ${XDG_CACHE_HOME:-$HOME/.cache}/gift/maps
mkdir -p "$dir"
go run github.com/protomaps/go-pmtiles@v1.31.2 extract \
    https://build.protomaps.com/20261031.pmtiles "$dir/oldenburg.pmtiles" \
    --bbox=8.05,53.05,8.35,53.23
```

Die taeglichen Builds bleiben nur eine Weile liegen; das aktuelle Datum steht
unter docs.protomaps.com/basemaps/downloads. Abgelegt ist der Ausschnitt unter
`os.UserCacheDir()/gift/maps/oldenburg.pmtiles`, auf dem Mac
`~/Library/Caches/gift/maps`, auf dem Pi `~/.cache/gift/maps`, damit Beispiele
und optionale Tests ihn auf beiden ohne Flag finden.

| Eigenschaft                 | Wert                                                                     |
|-----------------------------|--------------------------------------------------------------------------|
| Groesse                     | 14 MB, 1149 Kacheln, Zoom 0 bis 15, 58 Range-Requests in 5 s            |
| Inhalt                      | Protomaps Basemap 4.15.2, OSM-Stand 31.10.2026 04:00 UTC, MVT, gzip      |
| Ebenen                      | earth, water, landuse, landcover (bis z7), roads, buildings (ab z11), boundaries, places, pois |
| dichteste Kachel (z15, Innenstadt) | 4816 Objekte, davon 2797 Gebaeude und 1730 POIs; rund 50 000 Geometriewerte; 188 KB entpackt |

Die letzte Zeile ist die Grundlage fuer die Rasterzeit im Abschnitt
Vektorkacheln. Zum Selbsthosten genuegt jeder statische Server mit
Range-Unterstuetzung (nginx, Caddy, S3-kompatibler Speicher).

**Vorab zu beheben:** `asset.HTTP` setzt keinen User-Agent und sendet damit
den Go-Standard, den die OSM-Server blockieren. Die Richtlinie verlangt von
einem SDK einen sinnvollen eigenen Standard, der den SDK-Namen und eine URL
nennt, und eine einfache Ueberschreibung. Das betrifft jede HTTP-Quelle in
gift, nicht nur Karten.

### Die Kartennaht: Anbieter austauschbar

Ein Anbieter ist mehr als eine URL: Kachelart, Schema, Zoomgrenzen,
Abdeckung, Namensnennung und ob er Vorausladen erlaubt. All das steht in der
Naht, damit sich die Karte nach dem Anbieter richtet und nicht nach Annahmen:

```go
// Provider liefert die Kacheln einer Karte.
type Provider interface {
    Info() ProviderInfo
    // Tile liefert eine Kachel oder ErrNoTile, wenn es sie nicht gibt
    // (ausserhalb der Abdeckung, leeres Meer); andere Fehler gelten als
    // voruebergehend und werden wiederholt.
    Tile(ctx context.Context, t TileID) (Tile, error)
}

type ProviderInfo struct {
    Kind        TileKind // Raster oder Vector
    Schema      string   // bei Vektor: "protomaps-v4", "openmaptiles", ...
    MinZoom     int
    MaxZoom     int      // darueber wird die letzte Stufe vergroessert
    TileSize    int      // 256 oder 512 logische Pixel
    Bounds      geo.Rect // ausserhalb wird nichts angefragt
    Attribution string
    Prefetch    bool     // darf vorausgeladen werden; die OSMF verbietet es
}

type Tile struct {
    Data     []byte     // PNG, JPEG, WebP oder MVT, entpackt
    Format   TileFormat
    Revision string     // fuer den Cache; leer bei unveraenderlichen Archiven
}
```

| Anbieter                                           | Konstruktor              |
|----------------------------------------------------|--------------------------|
| PMTiles-Datei, selbst gehostet und offline (zuerst)| `maps.PMTiles(path)`     |
| PMTiles per HTTP-Range, selbst gehostet            | `maps.PMTilesURL(url)`   |
| XYZ-Vorlage: OSMF zum Entwickeln, MapTiler, Stadia, Thunderforest | `maps.XYZ(template, info)` |
| synthetisch, fuer Tests                            | `maps.DebugTiles()`      |

Ein PMTiles-Archiv nennt im Kopf, ob es Vektor- oder Rasterkacheln enthaelt;
`maps.PMTiles` meldet die Art danach, ein Raster-Archiv funktioniert also
ebenso.

**Der Stil haengt am Schema, nicht am Anbieter.** Vektoranbieter teilen OSM
als Quelle, aber nicht das Schema: Protomaps nennt seine Ebenen `roads` und
`earth`, OpenMapTiles (MapTiler) `transportation` und `landcover`. Ein
`maps.Style` erklaert, fuer welches Schema er gilt, und die Karte weist einen
Stil fuer ein anderes Schema bei der Konstruktion zurueck, statt eine leere
Karte zu zeigen. Zuerst ein Stil fuer Protomaps v4, hell und dunkel nach dem
Theme von gift. Ein Wechsel zwischen Rasteranbietern kostet also nichts, ein
Wechsel zu einem Vektoranbieter mit anderem Schema einen zweiten Stil.

**Anschluss an die Pipeline.** Ein Adapter macht aus einer Kachel eine
`asset.Source` (ID aus Anbieter und z/x/y, Revision aus der Antwort), damit
Prioritaeten, Budgets und bandweise Uploads ohne Sonderweg gelten.
Rasterkacheln decodiert die Pipeline wie jedes Bild. Fuer Vektorkacheln
bekommt sie eine Decode-Stufe, die statt eines Bildformats MVT mit Stil, Zoom
und Dichte zu Pixeln rastert; Stil und Dichte gehoeren dann in den
Cache-Schluessel. Ob gerasterte Kacheln auf die Platte gehoeren, entscheidet
die Messung: Rastern gegen das Lesen von 1 MiB je Kachel von der SD-Karte.

### Koordinaten und Genauigkeit

- Web Mercator (EPSG:3857), Kacheln im XYZ-Schema, Breite begrenzt auf
  ±85,0511°.
- Weltkoordinaten als float64 im Einheitsquadrat. Eine Kachel hat 256 logische
  Pixel bei ganzzahligem Zoom; der Zoom ist stetig, float64, 0 bis etwa 20.
- float32 reicht nicht: bei Zoom 20 ist die Welt 2^28 Pixel breit, float32 hat
  24 Bit Mantisse. Gerechnet wird in float64 und erst relativ zur Kameramitte
  nach float32 gewandelt, dieselbe Regel wie fuer grosse Dokumentpositionen
  der Galerie (Abschnitt 10).
- In x wiederholt sich die Welt wie bei MapKit: Kacheln modulo 2^z, Marker in
  jeder sichtbaren Kopie.

### Kamera und Gesten

`Camera{Center, Zoom, Heading}`. Eine Neigung (Pitch) ist vorgesehen, aber
nicht Teil dieser Lieferung. Dazu `geo.Region` (Mitte und Spanne) fuer "zeige
diese Koordinaten mit diesen Raendern" wie `MKCoordinateRegion`.

Gesten aus `ui/gesture`, Physik aus `ui/motion`, beides aus Abschnitt 26:

- Verschieben: Drag folgt dem Schwerpunkt der Finger, der Wurf bremst wie
  UIScrollView.
- Pinch: Magnify mit der Ankerformel aus Abschnitt 26, in Weltkoordinaten. Der
  Weltpunkt unter dem Schwerpunkt bleibt unter dem Schwerpunkt. Ueber die
  Zoomgrenzen hinaus Rubberband auf log(Zoom) wie im Bildbetrachter; ein Pinch
  mit Schwung zoomt nach, wie bei MapKit.
- Drehen: Rotate, bisher erkannt, aber ungenutzt, setzt das Heading. Am
  Gestenende rastet es unter etwa 7° auf Nord ein. Ein Kompass erscheint,
  sobald das Heading nicht null ist; ein Tipp darauf dreht nach Norden.
- Doppeltipp: eine Stufe hinein um den Tipp. Zwei-Finger-Tipp: eine Stufe
  hinaus.
- Desktop: das Mausrad zoomt um den Zeiger. Ein Trackpad (`Event.Precise`)
  verschiebt und zoomt mit Strg oder Befehlstaste, weil GLFW das Trackpad-Pinch
  auf dem Mac nicht liefert (Abschnitt 26). Pfeiltasten verschieben, Plus und
  Minus zoomen.
- Programmatisch `SetCamera(c, animated)`. Grosse Spruenge fliegen nach van
  Wijk und Nuij (heraus, hinueber, hinein, wie `flyTo` bei Mapbox), kurze
  federn.

### Kacheln: sichtbare Menge, Ersatz, Budgets

- Die sichtbare Menge ist der Schnitt des Viewports mit dem Kachelgitter, nicht
  seine Bounding-Box. Bei gedrehter Karte ist der Viewport in Weltkoordinaten
  ein gedrehtes Rechteck, und die Bounding-Box enthielte bis zu doppelt so
  viele Kacheln. Gerechnet wird mit einem konvexen Polygon, damit eine spaetere
  Neigung (dann ein Trapez) denselben Code nutzt.
- Kachelstufe z = round(Zoom); die Kachel wird um 2^(Zoom−z) skaliert, also
  zwischen 0,71 und 1,41.
- Nie leer, nie flackernd: fehlt eine Kachel, zeichnet die Karte den
  naechsten fertigen Vorfahren als Ausschnitt (Clip und uebergrosse Bounds,
  wie `OpImage` es vorsieht) oder fertige Kinder. Beim Stufenwechsel bleibt
  die alte Menge stehen, bis die neue vollstaendig ist.
- Kacheln sind `asset.Source`s und laufen durch die vorhandene Pipeline:
  Prioritaet `Visible`, Disk-Cache mit ETag, Decode-Worker, Upload-Budget und
  bandweise Uploads. Prefetch (ein Ring um den Viewport, die naechste Stufe in
  Pinch-Richtung) nur fuer Quellen, die es erlauben; fuer die OSMF ist er aus.
- Texturbudget mit LRU nach letzter Verwendung; eine im laufenden Frame
  gezeichnete Kachel wird nie verdraengt, dieselbe Regel wie im Glyphenatlas.
  Eine 256²-Kachel sind 256 KiB, bei Dichte 2 sind es 1 MiB. Full HD bei
  Dichte 1 zeigt etwa 40 Kacheln; 128 im Budget sind 32 MiB.
- Fuer Rasterkacheln braucht der Render-Vertrag nichts Neues: `OpImage` mit
  Transform und Clip genuegt.
- Risiko Naehte: bei gebrochenem Zoom und Drehung zeigen benachbarte Quads mit
  linearer Filterung haarfeine Kanten. Gegenmittel, im ersten Schritt per
  Golden zu entscheiden: die Abtastung auf die eigenen Texel begrenzen und die
  Ecken einer Stufe gemeinsam rechnen, oder alle Kacheln einer Stufe in eine
  Ebene zeichnen und diese drehen.

### Vektorkacheln

Aus dem PMTiles-Archiv, ohne neue Abhaengigkeit:

- PMTiles v3 lesen: 127 Byte Kopf, gzip-komprimierte Verzeichnisse, Varints,
  Kachel-IDs entlang der Hilbert-Kurve. Quelle eine lokale Datei oder
  HTTP-Range-Requests.
- MVT ist Protobuf; dafuer ein kleiner eigener Decoder. gift hat vier direkte
  Abhaengigkeiten, eine Protobuf-Bibliothek fuer drei Nachrichtentypen waere
  unverhaeltnismaessig.
- Zeichnen zuerst auf der CPU: jede Kachel wird in einem Worker mit dem
  Rasterizer der Icons (`x/image/vector`) in Geraeteaufloesung gerastert und
  laeuft danach als Bild durch dieselbe Pipeline wie eine Rasterkachel, mit
  Budgets, Cache und bandweisem Upload. Kein neuer Render-Op.
- Beschriftungen werden nicht in die Kacheln gebacken, sondern live gesetzt:
  aufrecht bei jeder Drehung, mit Kollisionserkennung nach Prioritaet und
  Einblenden wie bei MapKit. Zuerst Punktbeschriftungen (Orte, POIs) und gerade
  Strassennamen, gebogene spaeter.
- Die Alternative ist ein Render-Op fuer Dreiecke mit einmal je Kachel
  triangulierten Flaechen (earcut): scharf bei jedem gebrochenen Zoom, aber
  eine Erweiterung des Vertrags. Entscheidung nach Messung an der dichtesten
  Oldenburger Kachel (z15, 4816 Objekte): rastert der Pi 5 sie in unter etwa
  15 ms je Kern, bleibt es beim Rastern.

### Punkte, Cluster und Fotos

Wie bei der Galerie besitzt die Anwendung ein langlebiges Objekt, und die View
ist nur die Deklaration darauf: `maps.NewPoints(...)` haelt Index und Cluster,
`maps.View(m).Points(p, ...)` zeigt sie.

**Index und Verfahren.** Hierarchisches Greedy-Clustering nach Supercluster
(Mapbox): von der hoechsten Stufe abwaerts fasst jede ganzzahlige Zoomstufe
die Eintraege der Stufe darueber innerhalb eines Radius in Bildschirmpunkten
zusammen. Die Position eines Clusters ist der gewichtete Schwerpunkt, und
jeder Eintrag merkt sich seinen Elternteil eine Stufe tiefer. Je Stufe ein
statischer k-d-Baum in flachen Arrays wie KDBush. Die Vorberechnung kostet
O(n log n) je Stufe, laeuft in einer Goroutine und wird versioniert
uebernommen wie das Galerie-Layout. Die Bereichsabfrage je Frame allokiert
nicht und haengt an ein Slice des Aufrufers an, der Vertrag aus Abschnitt 10.

**Begrenzte Zahl.** Bei einem Radius von 60 pt zeigt Full HD in der
Groessenordnung von 570 Clustern (Flaeche durch Radius zum Quadrat; das
Greedy-Verfahren garantiert den Abstand nicht exakt), meist weit weniger,
gleich ob der Katalog tausend oder eine Million Fotos hat. Gezeichnete Marker, Thumbnail-Anfragen und Texturen sind
damit von N unabhaengig; das ist der Performance-Gewinn des Clusterns.

**Stufenwechsel mit Hysterese.** Die Clusterstufe folgt dem Zoom, wechselt
aber erst 0,1 hinter der Grenze, damit ein Pinch, der auf der Grenze
verharrt, nicht flackert.

**Verschmelzen und Aufbrechen, animiert.** Weil jeder Eintrag seinen
Elternteil kennt, ist der Uebergang eindeutig. Beim Hineinzoomen starten die
Kinder an der Position des Elternteils und federn an ihre eigene; beim
Herauszoomen fliegen sie in den Elternteil, der am Ende erscheint. Eine
kritisch gedaempfte Feder von etwa 300 ms, `motion.Spring` wie bei der
Bildschirmtastatur. Interpoliert wird in Weltkoordinaten, damit der Uebergang
stimmt, waehrend der Pinch weiterlaeuft.

**Repraesentatives Foto.** Jeder Cluster zeigt ein Foto, berechnet bei der
Vorberechnung als Aggregat (Supercluster kennt map und reduce): das neueste
oder das mit der hoechsten Bewertung der Anwendung. Ein verschmolzener Cluster
zeigt so immer das Foto eines seiner Kinder; dessen Textur liegt beim
Verschmelzen schon bereit, und der Uebergang kann es morphen statt neu zu
laden.

**Darstellung wie in Fotos.** Abgerundetes Thumbnail mit weissem Rand und
Schatten, die Anzahl als Plakette oben rechts, ein einzelnes Foto ohne
Plakette. Die Thumbnails kommen ueber `ui.Picture` in der Vorschaugroesse der
Galerie und teilen sich deren Cache.

**Marker sind Views, gepoolt.** Die Anwendung baut den Inhalt eines Markers
(`func(maps.Point) gift.View`) wie eine `MKAnnotationView`. Die Karte bindet
Slots eines Pools um, statt zu mounten, und setzt die Positionen in ihrem
Layout, genau wie die Galerie ihre Kacheln (Abschnitt 10).

**Tippen.** Ein einzelnes Foto meldet `OnSelect(id)`. Ein Cluster zoomt
animiert auf die Stufe, auf der er zerfaellt (`getClusterExpansionZoom` bei
Supercluster), oder meldet `OnCluster(c)`, damit die Anwendung wie Fotos ein
Raster der enthaltenen Bilder zeigt; dafuer passt die Galerie ueber einer
Teilkollektion.

**POIs.** Derselbe Index; Marker als Nadel mit Symbol und Titel wie
`MKMarkerAnnotationView`. Titel ueberdecken sich nicht, der mit niedrigerer
Prioritaet weicht.

**GPS aus EXIF.** `asset` liest aus EXIF heute nur die Orientierung, im IFD0.
Neu: dem Zeiger auf das GPS-IFD (Tag 0x8825) folgen und Breite, Laenge und
Aufnahmezeit lesen, im selben Probe ohne zusaetzliche Ein- und Ausgabe;
`asset.Metadata` bekommt einen Ort. Koordinaten, die erst nach dem Probing
bekannt werden, kommen gebuendelt wie die Korrekturen der Galerie, und der
Punktindex wird dann versioniert neu aufgebaut. HEIC spaeter, siehe
Abschnitt 22.

### Nichts ausserhalb des Sichtbaren

Die Regel: kein Op, kein Request und kein Upload fuer etwas, dessen
Bildschirmgrenzen den Viewport nicht schneiden.

- Kacheln: exakter Schnitt mit dem Viewport-Polygon. Vorfahren als Ersatz
  werden auf ihren sichtbaren Teil geclippt.
- Marker und Beschriftungen: Indexabfrage, erweitert um die groesste
  Ausdehnung eines Markers, dann ein genauer Test mit den sichtbaren Grenzen
  des Markers einschliesslich Schatten und Plakette. Ein Marker, dessen
  Ankerpunkt draussen liegt, dessen Bild aber hineinragt, wird gezeichnet;
  einer knapp daneben nicht.
- Wetter: die Shader decken nur den Viewport ab. Partikel leben in einem
  Bereich relativ zum Viewport und werden neu gesetzt, wenn sie ihn verlassen.
- Pruefbar: ein `gifttest`-Helfer, der fuer jeden Op der Anzeigeliste prueft,
  dass seine transformierten Grenzen den Viewport schneiden, und Zaehler
  (`Map.Stats`: gezeichnete Kacheln, Marker, Abfragen) fuer Tests und
  `giftmetrics`.

### Wetter als parallaxe Ebenen

**Entschieden am 31.10.2026:** das Wettermodell ist wie die Karte hinter einer
Naht abstrahiert. DWD und GFS sollen beide moeglich sein; begonnen wird mit
dem genaueren DWD.

**Die Wetternaht.** Die Darstellung kennt keinen Anbieter, sondern normierte
Groessen auf einem regulaeren Lat/Lon-Gitter:

```go
package weather

// Provider liefert die Felder eines Wettermodells.
type Provider interface {
    Info() ProviderInfo
    // Fetch liefert die Felder des juengsten Laufs fuer Region, Zeitfenster
    // und Groessen.
    Fetch(ctx context.Context, q Query) (*Run, error)
}

type ProviderInfo struct {
    Name        string
    Coverage    geo.Rect      // ICON-D2: Deutschland und Nachbarn; GFS: die Welt
    Resolution  float64       // nominell, km
    Step        time.Duration // zeitlicher Abstand der Felder
    Horizon     time.Duration // Laenge der Vorhersage
    Cycle       time.Duration // Abstand der Modelllaeufe
    Variables   []Variable    // was der Anbieter liefert
    Attribution string
}

// Variable ist eine normierte Groesse: Bedeckung als Anteil von 0 bis 1,
// Niederschlag in mm/h, Wind in m/s, Temperatur in K.
type Variable uint8

const (
    CloudLow Variable = iota
    CloudMid
    CloudHigh
    PrecipRate
    PrecipKind // Regen, Schnee, Graupel
    Wind10m    // Vektor
    WindAloft  // Vektor in etwa 500 hPa; treibt die Wolken
    Temp2m
)
```

Jeder Anbieter schneidet sein Gitter auf die angefragte Region zu, rechnet es
bei Bedarf auf ein regulaeres Lat/Lon-Gitter um, normiert die Einheiten und
leitet selbst ab, was er nicht direkt liefert, etwa die Niederschlagsrate aus
aufsummiertem Niederschlag. Fehlt einem Anbieter eine Groesse, laesst die
Darstellung die Ebene weg, statt zu raten. Radar ist eine eigene, kleinere
Naht, `weather.Nowcast`: Niederschlag in Minutenschritten fuer die naechsten
ein bis zwei Stunden, wo es ihn gibt; danach uebernimmt das Modell.

|                         | DWD (zuerst)                              | GFS (danach)                                   |
|-------------------------|-------------------------------------------|------------------------------------------------|
| Modell                  | ICON-D2                                   | GFS                                            |
| Abdeckung, Aufloesung   | Deutschland und Nachbarn, 2,2 km          | weltweit, 0,25° (etwa 28 km)                   |
| Laeufe, Horizont        | alle 3 h, bis 48 h                        | alle 6 h, stuendlich bis 120 h                 |
| Wolken in drei Hoehen   | `clcl`, `clcm`, `clch`                    | `LCDC`, `MCDC`, `HCDC`                         |
| Niederschlag            | `tot_prec` (aufsummiert), `ww`            | `PRATE`, `CRAIN`, `CSNOW`                      |
| Wind                    | `u_10m`, `v_10m`; u, v in Druckflaechen   | `UGRD`, `VGRD` in 10 m und 500 mb              |
| Nowcast                 | Radar-Komposit RV, 5 min, 1 km            | keiner                                         |
| Bezug                   | opendata.dwd.de, GRIB2, bz2               | NOMADS-Filter nach Region und Groesse, GRIB2   |
| Nutzung                 | frei, auch kommerziell, mit Quellenangabe | gemeinfrei                                     |

Beide liefern GRIB2, also ein gemeinsamer Decoder in `weather`: regulaeres
Lat/Lon-Gitter (Vorlage 3.0), einfache Packung (5.0) und die komplexe Packung
mit raeumlicher Differenz, die GFS verwendet (5.3). Ob der DWD CCSDS packt
(5.42), was einen eigenen AEC-Decoder braeuchte, und welches Format das
tar-Archiv des Radars enthaelt, ist im ersten Wetterschritt zu pruefen.
Open-Meteo waere bequemer als beide, ist kommerziell aber nur mit Abo nutzbar,
und haengt bei Bedarf an derselben Naht.

**Felder.** `weather.Field` ist ein regulaeres Gitter mit Gueltigkeitszeit,
skalar (Bedeckung, Niederschlag) oder als Vektorfeld (Wind). Zwischen
Zeitschritten wird interpoliert, damit Vorhersage und Nowcast als Animation
abspielbar sind. Dekodiert wird ausserhalb der Frameschleife. Laeufe sind
unveraenderlich und werden auf der Platte gecacht; `weather.Sky` fragt nach
dem `Cycle` des Anbieters nach einem neuen. Auf die GPU kommen kleine
Datentexturen, etwa 512² RGBA8 mit tief, mittel, hoch und Niederschlag, eine
zweite fuer den Wind.

**Wolken.** Je Ebene ein Shader: die Bedeckung, bilinear aus dem Gitter des
Anbieters, moduliert ein fBm-Rauschen, das mit dem Hoehenwind treibt. Eine
weiche Schwelle macht daraus Dichte, die Lichtrichtung eine einfache
Schattierung. Gerechnet in halber oder viertel Aufloesung und weich
hochskaliert: Wolken sind weich, und die Fuellrate des V3D bei Full HD ist
knapp. Das Rauschen nutzt den Hash ohne Sinus aus 9a59e4f, damit Goldens auf
jeder GPU stimmen.

**Parallaxe.** Ueber der Karte steht eine gedachte Kamera in der Hoehe H. Eine
Ebene in der Hoehe h wird um H/(H−h) um die Viewportmitte skaliert und bleibt
an Weltkoordinaten verankert. Beim Verschieben laufen hohe Wolken schneller
als der Boden, beim Pinch wachsen sie staerker. Dazu Wolkenschatten: die
Dichte, versetzt um die Sonnenrichtung aus Zeit und Ort, dunkelt die Karte ab.
Der Versatz waechst mit der Hoehe der Ebene und ist der staerkste
Tiefenhinweis.

**Niederschlag.** Zwei Darstellungen, waehlbar. Die klassische Radarfarbskala
auf Bodenhoehe, informativ. Und eine atmosphaerische als prozeduraler Shader
ohne Zustand: Regenstriche oder Flocken, in Zellen gehasht, in einigen
Tiefen; Dichte nach Intensitaet, Neigung nach Wind, Schnee oder Regen nach
Niederschlagsart. Ohne Partikelzustand kostet das keine CPU.

**Wind.** Stroemungslinien wie bei earth.nullschool oder Windy: einige
tausend Partikel, auf der CPU durch das Vektorfeld bewegt (RK2, unter
0,1 ms), als verblassende Spuren gezeichnet. Dafuer fehlen dem Render-Vertrag
zwei Dinge: ein Op fuer Dreiecke oder Liniensegmente und ein ueber Frames
bestehendes Offscreen-Ziel fuer die Spuren; der Layer-Cache ist ein Cache und
kein Zustand. Der Dreiecks-Op ist dieselbe Erweiterung, die die Alternative
fuer Vektorkacheln braucht und spaeter Routen und Flaechen als Overlays. Er
lohnt sich einmal fuer alle.

**Qualitaetsstufen** wie bei Glass (Abschnitt 8): Aus; Reduziert mit
statischen Wolken ohne Rauschanimation in viertel Aufloesung, Radarfarben und
ohne Partikel; Voll; Adaptiv nach Messung. Animiertes Wetter haelt die
Frameschleife am Laufen. Ruht die Karte, laeuft das Wetter mit 30 statt
60 fps, ueber `Animate` mit Intervall wie die Bildschirmtastatur.

### API-Skizze

```go
tiles, err := maps.PMTiles(filepath.Join(cache, "gift/maps/oldenburg.pmtiles"))
if err != nil { ... }
m := maps.New(maps.Config{
    Tiles: tiles,               // oder maps.PMTilesURL, maps.XYZ, ...
    Style: maps.ProtomapsStyle, // passend zum Schema der Kacheln
})
m.SetCamera(maps.Camera{Center: geo.LatLon(53.14, 8.21), Zoom: 12}, false)

photos := maps.NewPoints(maps.Photos(collection), maps.Clustering{Radius: 60})
sky := weather.NewSky(dwd.ICOND2(), dwd.Radar()) // spaeter auch gfs.Global()

return maps.View(m).
    Points(photos, maps.PhotoMarker).
    OnSelect(func(id maps.ID) { /* Foto oeffnen */ }).
    OnCluster(func(c maps.Cluster) { /* Raster zeigen oder hineinzoomen */ }).
    Weather(sky). // *weather.Sky
    Key("map")
```

| MapKit                                        | gift                                        |
|-----------------------------------------------|---------------------------------------------|
| `MKMapView`                                   | `maps.View` ueber `*maps.Map`               |
| `MKMapCamera`                                 | `maps.Camera`                               |
| `MKCoordinateRegion`                          | `geo.Region`                                |
| `MKAnnotation`                                | `maps.Point`                                |
| `MKAnnotationView`, `MKMarkerAnnotationView`  | Marker-Funktion, `maps.Marker`, `maps.PhotoMarker` |
| `MKClusterAnnotation`                         | `maps.Cluster`                              |
| `MKTileOverlay`                               | `maps.Provider`                             |
| `mapView(_:regionDidChangeAnimated:)`         | `OnCameraChange`, am Ende einer Geste       |

Pakete: `geo` (Koordinaten, Projektion, Region; ohne UI, wie `geom`),
`ui/maps` (Karte, Kartennaht, Kacheln, Stil, Punkte, Marker), `weather`
(Wetternaht, Felder, GRIB2, Darstellung) mit je einem Unterpaket pro Anbieter,
`weather/dwd` und spaeter `weather/gfs`, damit die Anbieter austauschbar
bleiben und eine Anwendung nur importiert, was sie nutzt. Dazu in `asset` der
Ort in `Metadata` und der Standard-User-Agent, im Render-Vertrag spaeter der
Dreiecks-Op und das bestehende Offscreen-Ziel.

### Schritte

| Schritt | Inhalt                                                                                         | Abnahme |
|---------|------------------------------------------------------------------------------------------------|---------|
| M1      | `geo`, Kamera, Gesten, sichtbare Menge, Ersatzkacheln, Budgets; die Kartennaht `maps.Provider` mit `maps.DebugTiles` | Anker bleibt unter dem Pinch; nichts ausserhalb (Helfer); Golden gedreht bei Zoom 3,4 ohne Naht; Pi 5, Full HD: Verschieben, Pinch, Drehen mit p95 ≤ 17,5 ms |
| M2      | Selbst gehostete Vektorkarte: `maps.PMTiles`, `maps.PMTilesURL`, MVT, Raster-Decode-Stufe, Protomaps-Stil hell und dunkel, Live-Beschriftungen, Namensnennung; Standard-User-Agent in `asset.HTTP` | Oldenburg offline auf dem Pi und per Range von einem lokalen Server; Rasterzeit der dichtesten Kachel gemessen und Entscheidung Rastern oder Dreiecke; Verschieben und Pinch mit p95 ≤ 17,5 ms |
| M3      | Punktindex, Clustering, gepoolte Marker, Uebergaenge, Fotomarker, GPS aus EXIF, Tippen          | 100 000 Fotos: Vorberechnung auf dem Pi im Hintergrund unter 0,5 s, Abfrage unter 50 µs ohne Allokation; Pinch ueber alle Stufen mit Thumbnails, p95 ≤ 17,5 ms |
| M4      | Zweiter Kartenanbieter als Beweis der Naht: `maps.XYZ` fuer Raster, Pruefung des Stils gegen das Schema, kein Prefetch fuer die OSMF | Wechsel zwischen PMTiles und XYZ allein ueber die Konfiguration; Handtest gegen OSM; ein zweiter Besuch laedt nichts neu |
| M5      | Wetter mit dem DWD: Wetternaht, GRIB2, ICON-D2, Radar RV, Wolken mit Schatten, Niederschlag, Wind mit Dreiecks-Op, Parallaxe, Stufen | Pi 5, Full HD, Stufe Reduziert mit p95 ≤ 17,5 ms; Stufe Voll gemessen |
| M6      | GFS als zweiter Wetteranbieter; ausserhalb der Abdeckung des DWD uebernimmt GFS (`weather.Combine`) | dieselbe Szene mit beiden Anbietern; kein Sprung am Rand der Abdeckung |
| M7      | Kitchen-Sink-Demo, Doku-Seiten                                                                 | — |

Die selbst gehostete Vektorkarte (M2) kommt direkt nach dem Fundament, weil
sie die entschiedene Quelle ist; danach die Fotos (M3) als Haupteinsatz. Der
zweite Kartenanbieter (M4) beweist die Naht, bevor sich zu viel auf PMTiles
eingeschliffen hat. Das Wetter zuletzt, weil es den Dreiecks-Op braucht, und
dort derselbe Beweis mit GFS als zweitem Anbieter (M6).

### Entscheidungen

Am 31.10.2026 entschieden:

1. Die Kartenquelle ist abstrahiert (die Kartennaht), der Anbieter wechselbar.
   Begonnen wird mit einem selbst gehosteten PMTiles-Ausschnitt von Oldenburg.
2. Das Wettermodell ist abstrahiert (die Wetternaht), DWD und GFS sind beide
   vorgesehen. Begonnen wird mit dem genaueren DWD.

Damit erledigt:

3. Drehen: Vektorkacheln mit Live-Beschriftung kommen in M2, vor jeder
   Rasterquelle; die Schrift bleibt bei jeder Drehung aufrecht. Ein
   Rasteranbieter (M4) dreht seine eingebackene Schrift mit; fuer ihn kann die
   Karte das Drehen abschalten, weil `ProviderInfo` die Kachelart nennt.

Weiter gilt:

4. Neigung, also echtes 3D, bleibt draussen; die sichtbare Menge ist dafuer
   vorbereitet.

Offen, aber erst fuer den Einsatz zu klaeren: ob der Ausschnitt im Produkt
auf dem Geraet liegt (Datei) oder auf einem eigenen Server (Range). Die Naht
kann beides.

### Risiken

- Das Rastern dichter Vektorkacheln ist auf dem Pi zu langsam; dann der
  Dreiecks-Op, der fuer den Wind ohnehin kommt.
- Die Fuellrate der Wolken auf dem V3D; dann Aufloesung und Stufen.
- GPU-Speicher: Kacheln, Thumbnails und Wetter teilen ihn und brauchen ein
  gemeinsames Budget.
- GRIB2: einfache (5.0) und komplexe Packung (5.3) sind sicher noetig, ob der
  DWD CCSDS (5.42) verwendet, ist ungeprueft, ebenso das Radarformat.
- Vektorschemata: ein Wechsel zu einem Anbieter mit anderem Schema braucht
  einen eigenen Stil. Das ist gewollt und in der Naht sichtbar, kostet aber
  Arbeit je Anbieter.
- Die taeglichen Protomaps-Builds verschwinden nach einiger Zeit. Ein
  geschnittener Ausschnitt gehoert auf eigenen Speicher, nie als Verweis auf
  `build.protomaps.com` in eine Konfiguration.
- Lizenzpflichten: ODbL-Namensnennung, Quellenangabe des DWD, Lizenz des
  Protomaps-Stils pruefen.

### Umsetzung und Messungen

Gebaut wie geplant, mit diesen Abweichungen und Ergaenzungen:

- **Pakete.** `geo`, `ui/maps`, `weather`, `weather/grib2`, `weather/dwd`,
  `weather/gfs`; im Render-Vertrag `render.ImageTile`, `OpTriangles` mit der
  Vertex-Seitentabelle; in `asset` `DefaultUserAgent`, `Pipeline.Post` und
  `ReadExif`; in `ui` `MeasureText`, `DrawText`, `Picture.DrawTile` und
  `Picture.DrawFit`; `gesture.WheelValue.Precise`;
  `PaintContext.AppendVertex`. Beispiel `cmd/example-maps`, Demo `map` der
  Kitchen Sink, Doku-Seiten "Map" und "Maps and Weather".
- **Naehte zwischen Kacheln.** Bei gebrochenem Zoom und Drehung zeigten die
  Kanten der Kacheln den Hintergrund: lineare Filterung mischt den Randtexel
  mit dem transparenten Rand der Textur. `render.ImageTile` tastet nur
  zwischen den Mittelpunkten der Randtexel ab; der GPU-Test bei Zoom 3,4 und
  17 Grad zaehlte vorher 1364 Fugenpixel, jetzt keines.
- **Vektorkacheln bleiben auf der CPU.** Die dichteste Kachel Oldenburgs
  (z15, 4816 Objekte) brauchte auf dem Pi 5 bei 256 Pixeln 23,6 ms; das
  Rastern in eine Alpha-Maske und Zusammensetzen nur der bedeckten Pixel
  brachte 11,1 ms, unter den 15 ms der Entscheidung. Der Dreiecks-Op war
  dafuer nicht noetig.
- **Beschriftungen** werden in einer eigenen Goroutine aus denselben
  Kacheln gelesen und ueber `asset.Pipeline.Post` uebergeben, weil ein Bild
  aus dem Cache kommen kann, ohne dass gezeichnet wird. Ein Frame shapt
  hoechstens zwoelf neue Namen; ein Schwenk in eine Strasse voller neuer
  Namen kostete vorher 14 ms Update, jetzt 1,2 ms.
- **Marker** sind keine gepoolten Views, sondern werden von der Karte selbst
  gezeichnet und im eigenen Tap-Erkenner getroffen; das spart den Build beim
  Schwenken. Sie werden in Ebenen ohne Ueberlappung Teil fuer Teil gezeichnet,
  was die Draw-Calls einer Fotokarte halbierte. Der Clusterradius ist 80 pt
  statt 60: bei 56 pt grossen Markern liessen 60 sie ueberlappen.
- **Gesten auf einem Kind.** Ein Knoten hat einen Timer, und der der Karte
  (naechstes Frame, Wiederholung, Einblenden) ersetzte den, mit dem ein Tap
  einen Doppeltipp ausschliesst. Gesten und Tasten liegen deshalb auf einer
  Eingabeflaeche ueber der Karte.
- **Wetter ohne neuen Shader.** Wolken, Schatten und Radar werden in einer
  Goroutine in Texturen gerechnet, wenn Lauf, Zoom oder Region wechseln, und
  nur, wenn die Karte ruht; im Frame sind sie Bilder mit Drift und
  Parallaxe. Wind und Niederschlag sind Partikel als `OpTriangles`; ein
  ueber Frames bestehendes Offscreen-Ziel war nicht noetig. Das Radar liegt
  ueber den Wolken, weil es unter einer geschlossenen Decke sonst
  verschwindet.
- **GRIB2.** DWD wie NOMADS-Filter liefern einfache Packung (5.0) mit
  Bitmap, beide am 31.10.2026 geprueft; 5.3 und CCSDS waren nicht noetig.
- **Zwei Fehler im Backend** zeigten sich an der Karte und betrafen jede
  Ansicht: Unter `DirectToScreen` vergass `BeginFrame` die beim Malen
  aufgeschobenen Uploads, und ein Bild, das auf ein Band wartete, galt als
  abgelehnt statt als aufgeschoben. Beide liessen eine Ansicht, die nur bei
  Bedarf zeichnet, mit fehlenden Bildern stehen. Behoben in 2ed5a14.
- **Flackern beim Zoomen und Verschieben** auf dem Mac (Dichte 2), Bild fuer
  Bild mit dem `capture` der Automatisierung gefunden; drei Ursachen:
  - Bereit hiess dekodiert, nicht hochgeladen. Nach einem Zoom fielen die
    Ersatzkacheln weg, sobald die neuen dekodiert waren, und die Karte war
    leer, bis das Upload-Budget sie Frame fuer Frame nachgeliefert hatte.
    Jetzt bleibt der Ersatz liegen, bis die Kachel einmal gezeichnet ist,
    und bevorzugt wird Ersatz, dessen Textur schon da ist. Das Beispiel
    laedt zwei Kacheln je Frame: 512 KiB unter Linux, sonst 2 MiB.
  - Die Schriftgroesse der Beschriftungen folgte dem Zoom stufenlos; jede
    neue Groesse wurde neu vermessen (zwoelf je Frame) und neu gerastert.
    Waehrend eines Zooms verschwanden fast alle Beschriftungen und blendeten
    danach wieder ein. Jetzt einmal vermessen und skaliert, in halben
    Punkten gestuft; Beschriftungen noch nicht gelesener Kacheln ersetzen
    die der Eltern- oder Kindkacheln, und eine kurz verdraengte
    Beschriftung blendet nicht neu ein. Fehlgriffe im Glyphenatlas der
    Pi-Messfolge: rund 410 statt rund 1000.
  - Eine neue Wetterberechnung verwarf die alte Textur vor dem Upload der
    neuen. Jetzt bleibt die alte bis dahin; beim Verschieben wird
    nachgerechnet, sobald die Ansicht dem Rand naeher kommt als ein
    Viertel ihrer Groesse, und die Raender laufen weich aus.
  Auf dem Pi unveraendert innerhalb der Streuung (Vektorkarte p95 17,4 ms
  wie vorher).
- **Schwarze Frames auf dem Mac**, das Flackern des ganzen Bildschirms, das
  danach blieb: Das `capture` zeichnet ueber den Offscreen und konnte es nie
  zeigen; eine Bildschirmaufnahme mit 60 Bildern je Sekunde zeigte Frames, in
  denen nur die Beschriftungen auf Schwarz standen. Ebitengines Metal-Treiber
  beginnt jeden Render-Pass auf den Bildschirm mit einem Loeschen, und ein
  Frame hat mehrere, sobald mitten darin etwas anderes geschrieben wird: eine
  Kachel hochgeladen, eine Glyphe in den Atlas gelegt. `DirectToScreen`
  zeichnet deshalb nur noch unter OpenGL direkt, sonst ueber den Offscreen.
  Danach in 540 aufgenommenen Frames keiner mehr dunkel; auf dem Pi (OpenGL
  ES, weiter direkt) unter 80 Bildschirmfotos waehrend acht Wuerfen keiner.
- **Kacheln, die nie kamen**: grau beim Herauszoomen, unscharf beim
  Hineinzoomen. Ein angekommenes Bild galt im Bilddienst fuer immer als
  bereit und wurde nie wieder angefordert, auch wenn seine Textur verdraengt
  war und seine Pixel aus dem CPU-Cache ebenfalls, oder die Pixel verdraengt
  wurden, bevor das Upload-Budget sie hochgeladen hatte. Nachgestellt: auf
  Stufe 11, dann auf 15 zwoelf Wuerfe, zurueck auf 11, und die Karte blieb
  grau. Jetzt verliert ein solches Bild den Zustand bereit, und das naechste
  Layout fordert es neu an. Nebenbei: Ein bereites Bild sammelte bei jedem
  Layout den Invalidator des Anfragenden, ohne dass ihn je jemand leerte.
  Das Beispiel gibt dem CPU-Cache ausserhalb von Linux 256 MiB statt 64,
  weil eine Kachel bei Dichte 2 1 MiB gross ist.
- **Ruckeln**: Ein Wurf von 2,2 s bekam auf dem Mac 141 Bildwechsel, davon 14
  ohne Update, die das Bild davor noch einmal zeigten, und 8 mit zweien; auf
  dem Pi 11 ohne und 10 mit zweien. Die Bildwechsel kamen puenktlich, aber
  Ebitengine zaehlt die Ticks eines Bildwechsels gegen die Uhr und rundet,
  und sechzig Bildwechsel zu sechzig Ticks liegen genau auf der Grenze.
  Ohne gesetzte `TPS` laeuft das Backend jetzt ein Update je Bildwechsel
  (`SyncWithFPS`); die Zeit von gift ist die Wanduhr und nicht der Tick, im
  Leerlauf gilt `IdleTPS` wie bisher. Danach auf beiden Geraeten jeder
  Bildwechsel eines Wurfs mit genau einem Update gezeichnet.
- **Beschriftungen am Rand** verschwanden, sobald ihr Kasten den Kartenrand
  beruehrte, und Kandidaten kamen nur aus Kacheln im Bild. Jetzt verlassen sie
  die Karte ueber den Rand, angeschnitten wie die Karte darunter, und die
  Kandidaten kommen aus allen Kacheln, die das um `labelReach` vergroesserte
  Sichtfeld schneidet.
- **Jenseits eines regionalen Archivs**: Ein PMTiles-Ausschnitt haelt auf jeder
  Stufe die ganzen Kacheln, die seine Region beruehren; eine grobe Stufe zeigt
  deshalb weiter ueber den Rand hinaus als eine feine. Die Karte fragte
  Kacheln ausserhalb nicht an, und beim Hineinzoomen verschwand, was die
  groebere Stufe gezeigt hatte, etwa das Zwischenahner Meer westlich von
  Oldenburg. Jetzt wird eine Kachel ausserhalb aus der Geometrie ihres
  naechsten Vorfahren innerhalb gezeichnet, wie oberhalb der letzten Stufe,
  bis zur Kachel der Stufe 0, die die ganze Erde zeigt; eine Rasterkachel
  steht dort vergroessert ein. Eine Reichweite von vier Stufen verschob die
  Grenze nur: Spanien teilt mit Oldenburg nur die Kachel der Stufe 0 und war
  auf Stufe 4 da, auf 5 nicht. Begrenzt ist der Ueberzoom auf vierzehn
  Stufen, bis wohin der Rasterer mit float32 auf einen halben Pixel genau
  zeichnet.

Raspberry Pi 5, Vollbild 1920 x 1080, Gestenfolge ueber die
Automatisierung (vier Wuerfe, sechs Raststufen des Mausrads), Kiosk-
Einstellungen, zwei Worker, Upload-Budget zwei Kacheln je Frame:

| Szene                                              | Frame-Intervall p95 |
|----------------------------------------------------|---------------------|
| Vektorkarte Oldenburg, Zoom 14,5, um 30 Grad gedreht | 17,4 ms           |
| dieselbe, Zoom 15,5 (ueber die letzte Stufe hinaus) | 17,4 ms            |
| 2000 Fotos, Zoom 13                                 | 17,4 ms            |
| 2000 Fotos, Zoom 15, gedreht                        | 18,9 ms            |
| Wetter des DWD, nur Wind                            | 17,3 ms            |
| Wetter des DWD, geschlossene tiefe Wolkendecke      | 19 bis 21 ms       |

Index von 100 000 Fotos: 264 ms im Hintergrund, Abfrage eines Full-HD-
Ausschnitts 3,6 Mikrosekunden ohne Allokation. In einem Fenster statt im
Vollbild schafft der Pi bei 1920 x 1080 keine 60 Hz, gleich mit welchem
Inhalt: der Compositor setzt das Fenster zusammen.

### Offen

- Das Wetter unter geschlossener Bewoelkung haelt die 17,5 ms nicht; ohne
  neues Vorrechnen erreicht es beim Festhalten 58 Bilder je Sekunde, und
  woher die Spitzen der Messfolge kommen, ist nicht geklaert.
- Fotos bei Zoom 15 gedreht: 104 Bild-Draw-Calls je Frame. Ein Texturatlas
  fuer gleich grosse kleine Bilder, Kacheln wie Vorschaubilder, ist der
  Kandidat.
- Radar-Nowcast des DWD (RV, polarstereographisches RADOLAN-Gitter) ist
  nicht gebaut; der Niederschlag kommt aus dem Modell.
- Nicht gebaut: Zoom-Schwung nach einem Pinch, Zwei-Finger-Tipp zum
  Herauszoomen, gebogene Strassennamen, Symbole fuer POIs, Clustering ueber
  den Antimeridian, Neigung.

### Quellen

- OSMF Tile Usage Policy: https://operations.osmfoundation.org/policies/tiles/
- OSM-Namensnennung: https://osmfoundation.org/wiki/Licence/Attribution_Guidelines
- PMTiles v3: https://github.com/protomaps/PMTiles/blob/main/spec/v3/spec.md
- Protomaps-Builds: https://docs.protomaps.com/basemaps/downloads
- PMTiles-Werkzeug: https://github.com/protomaps/go-pmtiles
- Mapbox Vector Tile: https://github.com/mapbox/vector-tile-spec
- Supercluster: https://github.com/mapbox/supercluster, KDBush: https://github.com/mourner/kdbush
- J. J. van Wijk, W. A. A. Nuij: Smooth and efficient zooming and panning, InfoVis 2003
- DWD Open Data: https://opendata.dwd.de/, Nutzungsbedingungen: https://www.dwd.de/copyright
- GFS ueber NOMADS: https://nomads.ncep.noaa.gov/
- Hash ohne Sinus: https://www.shadertoy.com/view/4djSRW

## 38. Trackpad-Momentum unter macOS

Gemeldet (01.11.2026): mit Schwung gescrollt schiesst besonders die Liste weit
ueber das Ende hinaus und bleibt dort viel zu lange.

- Ursache: Ebitengines Cocoa-Port liest wie GLFW nur `scrollingDeltaX/Y`,
  nimmt praezise Deltas mal 0,1 und verwirft `phase`, `momentumPhase` und
  jedes Event ohne Delta, also auch das Abheben. Das Momentum, das macOS nach
  dem Abheben schickt, kam bei gift als Finger an, der noch eine Sekunde
  weiterzieht: `wheelBounce` dehnte damit wie unter einem Finger. Gemessen im
  Kitchensink (Viewport 676 pt): bis 280 pt ueber den Anfang gezogen, rund
  1,3 s gehalten, Rueckfeder erst 120 ms nach dem letzten Momentum-Delta. Ein
  eigener Fling lief nicht zusaetzlich; ein praezises Rad stoppt ihn.
- Entscheidung: das Backend liest die Phasen selbst, mit einem lokalen
  NSEvent-Monitor (purego/objc, in `Run` auf dem Hauptthread installiert), und
  nimmt Delta und Phase aus demselben Event; `eb.Wheel` bleibt Rueckfall,
  solange der Monitor nichts gesehen hat. Neu: `App.PointerWheelPhase`,
  `WheelPhase` (Contact, Lift, Momentum) und `Event.Phase`.
- Physik wie AppKit, das WebKit im `ScrollingEffectsController` nachbaut:
  Finger dehnen mit (1 − 1/(x·0,55/d + 1))·d und halten die Dehnung bis zum
  Abheben; das Abheben federt sofort zurueck und verwirft das folgende
  Momentum; Momentum, das an ein Ende stoesst, prallt mit seiner
  Geschwindigkeit ab, x(t) = (x0 + 0,31·v·t)·e^(−12,5·t), und der Rest wird
  geschluckt, auch nicht an einen aeusseren Container weitergereicht.
- Lokal gegen AppKit (macOS 27.2) per `dlsym` nachgemessen:
  `_NSElasticDeltaForTimeDelta` ist genau (x0 − 0,31·v·t)·e^(−12,5·t), mit
  umgekehrtem Vorzeichen der Geschwindigkeit, und
  `_NSHyperbolicElasticDeltaForReboundDelta(x, c, d)` genau gifts `rubber`.
  Ergebnis am Bildschirm: 26 pt Ausschlag bei rund 3000 pt/s, nach etwa
  0,45 s zurueck.
- Unveraendert: ohne Phasen (Linux, Pi, Windows) dehnt ein Trackpad weiter,
  bis die Deltas aufhoeren; Touch federt weiter mit ω = 14.
- Offen: Praezisions-Touchpads unter Windows liefern ebenfalls Traegheit ohne
  Phasen. Systemweit gepostete Events (`CGEventPost`) brauchen das Recht der
  Bedienungshilfen; geprueft wurde mit `postEvent:` in den eigenen Prozess.
- Quellen: WebKit `ScrollingEffectsController.mm`, `NSScrollViewSPI.h` und
  der alte `ScrollElasticityController.mm`; Chromium
  `elastic_overscroll_controller_exponential.cc`; Apple-Doku zu
  `NSEvent.phase` und `momentumPhase`; GLFW-Issue 1429.

## 39. Listen: Auswahl, Hover und Trennstriche

Gemeldet (01.11.2026): die Liste der Hauptnavigation im Kitchensink sieht mit
Hover, Auswahl und Trennstrichen zusammen nicht harmonisch aus.

- Befund: die Navigation ist `ui.List` aus `ui.Row` und `ui.Section` in einem
  `ui.VScroll`. Der Hover war eine randvolle, eckige Flaeche in
  `ColorControlHover` (hell 244/246/250 auf Weiss, kaum zu sehen; dunkel ein
  kraeftiges Blaugrau), die Auswahl eine VStack-Huelle im Kitchensink mit 14 %
  Akzentfarbe und Radius 8, und die Trennstriche liefen an beiden entlang.
- Original gemessen, SwiftUI `List` mit `.sidebar` unter macOS 27.2, aktives
  Fenster: keine Trennstriche; Auswahl neutral grau (hell 228 → 206, dunkel
  73 → 99), 10 pt Einzug je Seite, volle Zeilenhoehe, Radius etwa 6 pt; Label
  fett, Symbole in Akzentfarbe; kein Hover. Recherche: UIKit blendet die
  Trennstriche um eine hervorgehobene Zelle aus; iPadOS-Sidebars haben keine;
  macOS-`.inset`-Tabellen runden die Auswahl und ruecken die Trennstriche auf
  die Textkante ein.
- Entscheidung: `RowView.Selected`; Hover, Druck und Auswahl sind eine Form,
  gerundet mit `RowCornerRadius` (8 bei 44-pt-Zeilen), neutral als Label-Farbe
  mit Alpha (hell 5/10/16 %, dunkel 7/14/20 %), die Auswahl bleibt unter dem
  Zeiger. `ListView` zeichnet keinen Trennstrich neben einer Zeile mit Flaeche
  (`PaintContext.ChildInteraction`); der Platz bleibt. `Separators(false)`
  nimmt sie samt Platz weg. Die Kitchensink-Sidebar zeichnet keine sichtbaren
  Trennstriche, behaelt aber ihren Punkt Fuge, damit Hover und Auswahl
  nebeneinander nicht verschmelzen; als Wurzel eines Navigationsstapels bleibt
  sie eine Liste mit Trennstrichen.
- Nicht uebernommen: das fette Label der Auswahl (die Standardschrift hat
  keinen familienunabhaengigen fetten Schnitt) und Symbole in Akzentfarbe.
- Nachgezogen: `VirtualList` hat dieselben Flaechen (`rowFaces`,
  `RowCornerRadius`). Eine Zeile zeichnet ihren Strich unten selbst; ob die
  naechste Zeile eine Flaeche hat, sammelt die Liste vor dem Malen der Zeilen
  per `ChildInteraction` je Slot ein (`ListState.lit`), weil die Slot-Reihenfolge
  nicht die Reihenfolge der Eintraege ist. Die Flaeche laesst den Punkt des
  Strichs frei, so bleibt zwischen zwei Flaechen eine Fuge wie in `ListView`.

## 40. Typst als Seiten-Engine (Durchstich)

Anlass (01.11.2026): Recherche zu PDF, Richtext und Typst (Bericht unter
`reports/`, nicht versioniert); gewuenscht war ein Durchstich mit Typst als
cgo-freie Shared Library.

- Aufbau: `typst/ffi` ist ein Rust-Crate (Typst 0.15.1, gepinnt) mit C-ABI,
  gebaut als `libgifttypst`; `typst` laedt sie wie libturbojpeg per purego,
  gift baut weiter mit `CGO_ENABLED=0`. Hinein gehen Schriftdateien und der
  Quelltext, heraus pro Seite eine flache Display-Liste in Punkt
  (Glyphenlaeufe, Rechtecke, Dreiecke, Bildflaechen) und auf Wunsch das PDF.
  Die Engine behaelt `World` und Quelltext, Typsts Memoisierung macht das
  Neusetzen inkrementell.
- Gezeichnet wird mit gifts eigenem Renderer (`ui/typstview`): Typsts Glyphen
  laufen durch gifts Glyphen-Atlas, weil `AddFont` dieselben Bytes auch in
  `internal/text` laedt; Glyphenursprung und duenne Linien werden auf ganze
  Geraetepixel gerastet, damit Tabellenlinien gleich stark bleiben.
- Jede Glyphe traegt den Byte-Offset ihres Textes im Quelltext (-1 fuer
  Erzeugtes wie Seitenzahlen). Daraus: Klick auf die Seite → Offset, Offset →
  Cursor. `cmd/example-typst` editiert damit direkt auf der gesetzten Seite;
  getippte Zeichen werden fuer Markup maskiert (`#` → `\#`), Enter erzeugt
  einen neuen Absatz.
- Gemessen (Apple M5, ein Zeichen in der Mitte, Neusetzen samt Uebergabe
  aller Seiten an Go): 1 Seite 0,08 ms, 5 Seiten 0,5 ms, 45 Seiten 6,3 ms,
  134 Seiten 34 ms. Die Bibliothek ist 34 MB gross (gestrippt).
- Nicht im Durchstich: Bilder, Dateien und Pakete fuer Typst, Verlaeufe,
  Clips, gedrehter Text, nicht-konvexe Fuellungen, nur geaenderte Seiten
  uebergeben, IME, Auswahl, Undo, Bedienungshilfen, Bidi beim Cursor.
- Gemessen auf dem Raspberry Pi 500 (BCM2712 wie der Pi 5, Trixie): Typst
  verlangt Rust 1.92, Trixie bringt 1.85, gebaut wurde deshalb mit Rust 1.99
  per rustup. Der Build dauert dort gut 7 Minuten, die Bibliothek ist 36 MB
  gross, alle Tests laufen. Ein Zeichen in der Mitte kostet bei 1 Seite
  0,2 ms, 5 Seiten 1,4 ms, 45 Seiten 20 ms, 134 Seiten rund 100 ms, das Drei-
  bis Vierfache des M5. Bei 45 und 134 Seiten liegen rund 70 % der Zeit in
  Rust (Typst samt Kodieren aller Seiten) und knapp 20 % im Dekodieren aller
  Seiten in Go; nur geaenderte Seiten zu uebergeben spart also hoechstens
  ein Viertel, der Rest ist Typsts Neusetzen.
- Das Beispiel auf dem Pi 500 (Fenster 1280 x 860 unter labwc, 3 Seiten mit
  zwei Tabellen, 48 Zeichen im Abstand von 110 ms auf der Seite getippt):
  Neusetzen p50 7,1 ms, p95 9,0 ms, einmal 48 ms; Frame-Intervall p50
  16,7 ms, p95 17,2 ms, p99 18,3 ms, ein Frame verpasst; Draw-CPU p95
  2,0 ms. Bereit 0,26 s nach dem Start, das erste Setzen 42 ms.
- Eingebettet (01.11.2026): `typst/embedded` traegt die Bibliothek fuer
  macOS arm64 und Linux amd64 und arm64, gzip-komprimiert (13,6, 14,1 und
  14,8 MB, zusammen 42,5 MB im Repository und in jedem Modul-Download).
  Wer sie will, importiert das Paket fuer den Seiteneffekt; eingebettet wird
  nur die Bibliothek der Zielplattform (das Beispiel waechst von 24 auf
  38 MB), ohne den Import zahlt eine Anwendung nichts. Beim ersten Gebrauch
  wird sie nach `os.UserCacheDir()/gift/typst/<Pruefsumme>/` entpackt
  (M5 0,17 s, Pi 500 0,4 s), danach nur noch geladen. Reihenfolge der
  Suche: `Open(path)`, `GIFT_TYPST_LIB`, eingebettet, neben dem Programm,
  Suchpfad des Linkers.
- `typst/ffi/build.sh` baut alle drei mit der in `rust-toolchain.toml`
  gepinnten Toolchain (Rust 1.99.1 per rustup), Linux per cargo-zigbuild
  gegen glibc 2.28 (Debian 10, Ubuntu 20.04, RHEL 8 und spaeter); auf dem
  M5 dauert das gut 4 Minuten. Die Linux-Bibliotheken haengen nur an libc,
  libm, libpthread und libdl. Cross- und nativ gebaute Bibliothek sind auf
  dem Pi 500 gleich schnell (45 Seiten je 20 ms). Gzip statt zstd (10,5 MB)
  oder xz (9,3 MB), weil beide eine neue Abhaengigkeit braeuchten.
- Geprueft auf macOS arm64 und dem Pi 500 (Tests und Beispiel) und in
  Docker: Debian 10 (glibc 2.28) auf amd64 und arm64, Ubuntu 26.04
  (glibc 2.43) auf amd64, jeweils alle Tests von `typst` und
  `typst/embedded`; amd64 lief dabei unter Rosetta.
- Offen: iOS braucht einen eigenen Weg
  (Framework im App-Bundle oder statisch gelinkt); signierte macOS-Apps mit
  Hardened Runtime muessen die Bibliothek signiert im Bundle mitbringen.

## 41. Typst-Dokumente auf der Seite editieren, Code-Editor

Anlass (01.11.2026): gewuenscht waren eine einfach zu benutzende Richtext-
Komponente zum Editieren und Anzeigen, die ganz auf Typst als Format beruht,
die PDF-Erzeugung in der Kitchen Sink und ein Code-Editor mit Zeilennummern und
Syntaxfarben, den das Typst-Beispiel nutzt und der Fehlerzeilen rot markiert.

- `ui.CodeEditor` ist ein `TextArea` ueber demselben `TextEditor`; der Knoten
  des Areas bekommt einen optionalen `codeState` (Gutter in der linken
  Polsterung, Farben je Token, Marken), so dass Auswahl, Scrollen und Eingabe
  dieselben bleiben. `ui.Highlighter` liefert `ui.Token`s von zwoelf Arten,
  Farben nach Xcode hell und dunkel. `CodeMark` (Byte-Bereich) toent die
  beruehrten Zeilen, unterstreicht den Text und faerbt die Zeilennummer.
  Tab rueckt ein (Zeile oder Auswahl), Shift-Tab aus, Enter behaelt die
  Einrueckung, Escape und dann Tab verlaesst den Editor. Keine Tabstopps: ein
  Tab ist ein Zeichen breit und wird leer gezeichnet (der Shaper haette ein
  Kaestchen).
- `ui/syntax` (eigenes Paket, damit `ui` keinen Lexer traegt): `syntax.Typst`
  (ein Durchlauf mit Modus-Stapel fuer Markup, Code und Mathe; 10 KB in
  24 Mikrosekunden ohne Allokation) und `syntax.Go` (go/scanner). Gefuzzt.
- `ui/typstview`: `Document` haelt Quelltext, Caret und Auswahl und setzt auf
  einer eigenen Goroutine; Ergebnisse kommen ueber ein `gift.Token` der
  beobachtenden Komponente zurueck (`Watch`). `Editor` (Werkzeugleiste und
  editierbare Seiten) und `Viewer`. Jede Bearbeitung auf der Seite ist eine
  Bearbeitung des Quelltexts: getippte Markup-Zeichen werden maskiert (in
  Strings anders als im Markup), Loeschen entfernt nur Gezeigtes und laesst
  Markup stehen, Fett/Kursiv mit `*`/`_` an Wortgrenzen und `#strong[]`/
  `#emph[]` im Wort. Ein Index der Glyphen traegt Bearbeitungen seit dem
  letzten Satz nach und zaehlt getippten, noch nicht gesetzten Text als
  sichtbar, sonst loeschte Backspace kurz nach dem Tippen das falsche
  Zeichen. Typst zeigt bei Escapes wie `\#` auf den Backslash; der Index nimmt
  die ganze Sequenz.
- Schriften: `text.Font` behaelt seine Bytes, `text.Find` findet eine schon
  geladene Schrift an denselben Bytes wieder, `ui.Font.Data` gibt sie heraus.
  Ein Dokument bekommt alle in gift registrierten Schriften, ohne zweite
  Font-ID. Typst liest nur TTF/OTF; IBM Plex Mono (WOFF) sieht es nicht, und
  `font/inter` bringt keine Kursive mit, deshalb bleibt `_kursiv_` aufrecht.
- Kitchen Sink: `code-editor` (Basic, Go mit einem Tippfehler als Marke) und
  die neue Gruppe Documents mit `typst-editor` und `typst-pdf` (Viewer, PDF
  nach `os.TempDir()`, oeffnen mit open/xdg-open). Doku-Seiten, Screenshots
  und eine Beispielseite fuer `example-typst`; apigen kennt `ui/typstview`,
  `ui/syntax` und `typst`.
- `example-typst`: links `CodeEditor` mit `syntax.Typst` und den
  Diagnosen als Marken, rechts `typstview.Editor` auf demselben Dokument,
  in beide Richtungen synchron.
- Offen: Undo, Tabstopps, Bilder und Pakete fuer Typst, Caret im Viewer,
  Hugo-Build der Seite nicht geprueft (hugo fehlt auf dem Mac).

## 42. Kiosk: gift als Betriebssystem-Oberflaeche eines Geraets

Anlass (01.11.2026): eine optionale Erweiterung fuer Anwendungen, die auf
einem Kiosk oder Geraet laufen (Pi mit Display 5, 7 und 10 Zoll) und direkt
hinein booten. Alles liegt im gift-Monolithen: die Fachanwendung und das, was
sonst ein Betriebssystem waere, nach dem Vorbild der Einstellungen von iOS und
iPadOS. Vorlage und Gegenbeispiel ist eventprint: dort steckt der Kiosk in
Bash-Skripten (Autologin-Benutzer, labwc-Konfiguration, xhost, polkit,
Provisionierung bei jedem Boot), WLAN laeuft ueber nmcli mit dem Passwort in
argv, Ethernet, statische Adressen, Updates, Neustart und Systeminfos fehlen,
und zurueck zum Desktop fuehrt kein Weg. Recherche unter
`research_notes/Kiosk Systemeinstellungen/`.

### Entscheidungen

- Die Anwendung laeuft im Kiosk immer als root. Keine Installskripte: die
  Anwendung wird auf dem Desktop gestartet, laeuft dort als Fenster ohne
  Rechte (nur lesend) und bietet "Als Kiosk installieren" an; das erhebt sich
  mit pkexec, kopiert das Programm nach `/usr/local/bin/<name>` und richtet
  den Kiosk ein.
- Kiosk-Boot ohne Kiosk-Benutzer und ohne Eingriff in lightdm/gdm: eine eigene
  Unit `<name>-kiosk.service` mit `Alias=display-manager.service`, die als
  root auf tty1 (PAM-Session ueber logind) labwc mit eigener Konfiguration
  startet (`labwc -C /etc/<name>/labwc -S <programm>`); labwc liefert
  Xwayland fuer Ebiten, Touch mit Mausemulation und `wlr-randr`. Boot zum
  Desktop heisst: diese Unit aus, den Display-Manager der Distribution wieder
  an. Beides schaltet die Anwendung selbst, in den Einstellungen.
- Systemdienste ueber D-Bus (`github.com/godbus/dbus/v5`, reines Go):
  NetworkManager, hostname1, timedate1, login1, systemd1. Kein Parsen
  lokalisierter Ausgaben, keine Geheimnisse in argv. Jede Netzwerkaenderung
  laeuft unter einem NetworkManager-Checkpoint mit Rollback, den die
  Oberflaeche wie eine Bildschirmaufloesung bestaetigen laesst.
- Updates: apt-get mit `APT::Status-Fd` fuer Fortschritt, getrennt nach
  Anwendung (eine Schnittstelle, die die Anwendung liefert), System
  (`update` + `full-upgrade`) und Release-Upgrade (Ubuntu
  `do-release-upgrade`; Pi OS unterstuetzt das nicht, dort nur ein Hinweis).
  Jede Art ist per Code erlaubt oder verboten.
- Zielsysteme: Raspberry Pi OS trixie und Ubuntu 26.04 LTS (Desktop, mit
  NetworkManager); macOS nur lesend; ein simuliertes System fuer Tests, die
  Kitchen Sink und die Doku.
- Groessenklassen nach iOS: kompakt (5 Zoll und kleiner) als
  Navigationsstapel wie auf dem iPhone, regulaer (7 Zoll und groesser) mit
  Seitenleiste und Detail wie auf dem iPad; Schwelle 680 logische Pixel
  Breite. Die Dichte eines Panels folgt aus seiner physischen Groesse.
- PIN nach dem Vorbild der Bildschirmzeit: optional per Code oder in den
  Einstellungen gesetzt, schuetzt waehlbare Bereiche, Sperrzeiten nach der
  Tabelle von iOS; gespeichert als PBKDF2-Hash.

### Pakete

- `kiosk/system`: die Abstraktion des Betriebssystems, ohne Oberflaeche.
  Info, Speicher und Aufraeumen, Netzwerk, Updates, Strom, Start (Kiosk oder
  Desktop, Installation), Anzeige, Zeit. Backends `linux` (Pi OS, Ubuntu),
  `darwin` (lesend), `Simulated`.
- `kiosk`: der Rahmen. Er zeigt die Anwendung, oeffnet die Einstellungen,
  sperrt sie mit der PIN und traegt die Seiten der Anwendung ein.

### Stand (01.11.2026)

- `kiosk/system`: Schnittstelle `System` mit Info, Speicher und Aufraeumen
  (apt-get clean, Journal auf 50 MB, systemd-tmpfiles, nie /tmp leeren, dort
  liegen die Sockets von labwc und Xwayland), Netzwerk ueber NetworkManager
  (Schnittstellen, WLAN suchen, beitreten mit Erkennung des falschen
  Passworts ueber den StateReason, ein nie verbundenes Profil wird wieder
  geloescht, Vergessen, IPv4 automatisch oder fest unter einem Checkpoint
  mit 60 s Rollback; GetSettings laesst die Geheimnisse weg, sie werden vor
  Update geholt), Updates ueber apt-get mit Status-Fd und einem
  block-Inhibitor von logind, Release-Upgrade nur Ubuntu, Strom ueber
  logind, Hostname ueber hostnamed samt 127.0.1.1 in /etc/hosts, Zeit ueber
  timedated, Anzeige ueber wlr-randr und /sys/class/backlight, Installation
  und Bootart ueber systemd (EnableUnitFiles/DisableUnitFiles, Alias
  display-manager.service). Backends Linux, macOS (lesend), andere
  (fast nichts) und `Simulated`.
- `kiosk`: `Config`, `Kiosk`, `Run`. Einstellungen kompakt
  (NavigationStack) und regulaer (Seitenleiste 320 px plus Stapel), Seiten
  Info, Name, WLAN, Netzwerkdetail, anderes Netzwerk, Ethernet,
  Schnittstelle mit IPv4 (Maske als 255.255.255.0 oder Praefix), Allgemein,
  Softwareupdate (Programm, System, Release), Speicher (Balken nach
  Kategorie, Aufraeumen), Datum & Uhrzeit, Zeitzone, Start (installieren,
  Kiosk oder Desktop, jetzt wechseln), Anzeige (hell/dunkel, Helligkeit,
  Drehung), Code-Sperre (setzen, aendern, Bereiche) und die Seiten der
  Anwendung. PIN als PBKDF2 (200 000 Runden) in `<DataDir>/kiosk.json`,
  Sperrzeiten nach iOS, ueber Neustarts erhalten. Ohne root laufen
  Installation und Bootart ueber `pkexec /usr/bin/env GIFT_KIOSK_ACTION=...
  <programm>`; ein Argument wuerde am flag.Parse der Anwendung scheitern.
- `ui.Row.Leading` fuer die farbigen Kacheln, `backend.Config.Density` fuer
  eine Dichte des Programms (Kiosk: 2 bis 5,5 Zoll, sonst 1).
- `cmd/example-kiosk`: Werkstueckzaehler mit den Einstellungen, `-simulate`.
- Geprueft: Tests ohne Fenster gegen `Simulated` (oeffnen, beide Layouts,
  falsches WLAN-Passwort, feste Adresse mit Behalten, PIN, Updates), Parser,
  erzeugte Dateien, `-race`. Auf dem Pi 500 lesend als root (Info,
  Speicher, Schnittstellen, WLAN-Suche 3,8 s, Uhr, 485 Zeitzonen) und das
  Beispiel als Fenster im Desktop, ohne root, beide Layouts. macOS lesend.
- Auf dem Pi 500 (01.11.2026, mit Erlaubnis): als root installiert (1,1 s,
  lightdm aus, Kiosk-Unit an, Alias umgehaengt), sofort in den Kiosk
  gewechselt (bereit nach 1,4 s: labwc als root auf tty1, Xwayland, die
  App vollflaechig, Einstellungen mit root-Rechten), ueber Einstellungen,
  Start, "Switch to the Desktop Now" zurueck (lightdm, VT 7, Alias, die
  Konsole auf tty1), danach alles wieder entfernt. Zwei Befunde dabei: die
  Sitzung auf tty1 wurde nicht aktiv, solange der Desktop auf VT 7 vorn war,
  und labwc wartete auf den Sitz (jetzt `ExecStartPre=-/usr/bin/chvt 1`);
  der Kiosk nahm getty@tty1 per Conflicts weg und der Rueckweg holte ihn
  nicht zurueck (SetBoot startet ihn jetzt). Ausserdem lehnte labwc die
  leere Titelleiste in rc.xml ab.
- Nicht geprueft: schreibende Netzwerkpfade gegen NetworkManager (eth0 des
  Testgeraets haengt im MSR-Netz), ein echter apt-Lauf, Installation ueber
  pkexec mit Passwortdialog, ein Neustart in den Kiosk, Ubuntu 26.04.
- Offen: WLAN-Land auf dem Pi (rfkill bis gesetzt), Bildschirmabschaltung
  nach Leerlauf, Ubuntu Server ohne NetworkManager, Anzeige in der Doku mit
  Screenshots.

## 43. Native Benachrichtigungen

Anlass (01.11.2026): Benachrichtigungen ausserhalb des Programmfensters, wie
sie auch der Sperrbildschirm zeigt. Ziel sind macOS, Windows 11 und Linux mit
GNOME und KDE; ein System ohne Unterstuetzung tut nichts. Windows steht damit
zum ersten Mal auf der Liste, entgegen Abschnitt 1 und der Begruendung in
`clipboard`; das braucht eine Windows-Testmaschine.

### Der gemeinsame Nenner

| Funktion | macOS | Windows 11 | GNOME | KDE |
|---|---|---|---|---|
| Titel und Text, Klartext | ja | ja | ja | ja |
| Name und Icon aus der Registrierung | Bundle | AUMID | desktop-entry | desktop-entry |
| Ersetzen und Zurueckziehen per ID | identifier | Tag | replaces_id | replaces_id |
| Klick, solange die App laeuft | Delegate | Activated | Aktion "default" | Aktion "default" |
| Knoepfe | nur vorab registriert | ja | ja | ja |
| Bild, Ton, Dringlichkeit, Ablauf | uneinheitlich | | | |
| Klick nach Programmende | automatisch | COM-Aktivator | D-Bus-Aktivierung | |

`notify` verspricht genau die ersten vier Zeilen. Der Sperrbildschirm ist
ueberall Einstellung des Benutzers; KDE Plasma zeigt dort nach heutigem Stand
gar keine Inhalte.

### Linux: ein eigener D-Bus-Client

Die Desktop Notifications Specification ueber den Session-Bus, die GNOME
Shell, KDE Plasma und die kleinen Server (dunst, mako, xfce4-notifyd)
umsetzen. Ohne cgo gab es drei Wege: godbus/dbus (reines Go, aber das erste
Modul, das dieses Projekt fuer ein Plattformmerkmal aufnimmt), libdbus per
purego (Callbacks und eine eigene Main-Loop) oder ein eigener Client.
Entschieden ist der eigene, weil der Bedarf klein ist: eine Verbindung,
EXTERNAL-Authentifizierung ueber einen Unix-Socket, fuenf Aufrufe, zwei
Signale.

- `wire.go`: alle Typen ausser UNIX_FD, beide Byte-Reihenfolgen beim Lesen,
  die Grenzen der Spezifikation (64 MiB je Array, 128 MiB je Nachricht,
  Tiefe 64). Getestet gegen von Hand ausgerechnete Byte-Layouts und gefuzzt.
- `conn.go`: Adresse (`DBUS_SESSION_BUS_ADDRESS`, sonst
  `$XDG_RUNTIME_DIR/bus`), SASL EXTERNAL, Hello, ein Lese-Goroutine fuer
  Antworten und Signale, `Peer.Ping` wird beantwortet. Jeder Aufruf hat zwei
  Sekunden.
- `freedesktop.go`: AddMatch vor GetCapabilities, Notify mit `desktop-entry`
  als Hint, die ID der Anwendung wird auf die Nummer des Servers abgebildet
  (`replaces_id`). Kann der Server Markup (`body-markup`, GNOME und KDE),
  wird der Text maskiert, sonst verschwaende bei "a < b" der Rest. Die
  Aktion "default" wird nur mit OnClick und nur bei der Faehigkeit
  `actions` gesendet. Ein Klick geht ueber `Config.Post` auf den
  UI-Executor.
- Alles ausser dem Oeffnen des Sockets ist ohne Build-Tag und laeuft auf dem
  Mac mit `go test ./notify/`, gegen einen Bus im Test ueber `net.Pipe`.
- Geprueft am 01.11.2026 in einem Container (Debian trixie, linux/arm64)
  mit `dbus-daemon` und dunst: Anzeigen, Ersetzen unter derselben Nummer,
  Klick per `dunstctl action` bis in OnClick, Zurueckziehen.
  `GIFT_NOTIFY_DBUS=1 go test ./notify/ -run DBus -v` wiederholt das auf
  einem echten Desktop.
- Nicht geloest: das Fenster nach vorne holen. GNOME und KDE schicken ein
  ActivationToken fuer xdg-activation, Ebitengine kann damit nichts
  anfangen. Flatpak und Snap brauchten stattdessen das Portal
  `org.freedesktop.portal.Notification`. Geht die Busverbindung verloren,
  wird nicht neu verbunden; jedes Show liefert dann einen Fehler und loggt
  ihn.

### macOS: umgesetzt (01.11.2026)

- `notify/platform_darwin.go`: UNUserNotificationCenter ueber purego/objc.
  Zuerst die Pruefung auf ein Bundle (`bundleIdentifier`, Pfad auf `.app`),
  sonst `ErrUnsupported` statt SIGABRT; `TestNotABundleIsUnsupported` laeuft
  in jedem `go test` auf dem Mac. Ein Delegate (`objc.RegisterClass`) zeigt
  Banner auch bei aktiver App und reicht den Klick auf die Nachricht ueber
  `Post` weiter; die Bloecke des Systems ruft `callBlock` ueber das
  Block-ABI auf, weil purego nur eigene Bloecke aufrufen kann. Vor jedem
  Show wird die Erlaubnis abgefragt, nicht gemerkt.
- Jedes Show ist eine neue Anfrage mit eigener Kennung; eine bereits
  gezeigte ID wird danach entfernt. Zwei Anfragen unter derselben Kennung im
  Abstand von 2 ms haben auf macOS 27.2 beide verloren (`CancellationError`
  in usernoted), obwohl das Ersetzen per Kennung der dokumentierte Weg ist.
- `cmd/giftbundle`: Wrapper fuer `go run -exec` und `go test -exec`. Bundle
  unter `~/Library/Application Support/gift/bundles/<id>.app`,
  `Info.plist` mit `NSHighResolutionCapable`, Ad-hoc-Signatur,
  `lsregister -f`, dann `exec`. Auf anderen Systemen startet es das Binary
  unveraendert.
- `cmd/example-notify`: Erlaubnis, Anzeigen oder Aktualisieren unter einer
  ID, Zurueckziehen, Klickzaehler.
- Der Klick kommt auf der Main-Queue an, die nur der Haupt-Thread bedient.
  Eine Ebitengine-App tut das (NSApplication), ein Testbinary nicht; der
  Test parkt deshalb den Haupt-Thread in `dispatch_main`. Ohne das meldete
  usernoted den Klick an den Prozess, und der Delegate sah ihn nie.
- Geprueft auf diesem Mac (macOS 27.2): Erlaubnisfrage, Zustellung als
  Banner, auf dem Sperrbildschirm und in der Mitteilungszentrale (laut Log
  von usernoted), Ersetzen, Klick bis in OnClick, Zurueckziehen.

### macOS: Ergebnis der Recherche

`UNUserNotificationCenter` ueber purego/objc ist moeglich (Blocks und
`objc.RegisterClass` fuer den Delegate sind da), verlangt aber eine
Identitaet, die ein nacktes Go-Binary nicht hat:

- Ein `.app`-Bundle mit `CFBundleIdentifier` (dazu `CFBundleExecutable`,
  `CFBundlePackageType=APPL`), bei LaunchServices registriert (`open` oder
  `lsregister -f`) und nicht unter `/var/folders`, wo `go run` baut.
  Ohne Bundle bricht schon `currentNotificationCenter` den Prozess mit
  SIGABRT ab; vorher muss `[[NSBundle mainBundle] bundleIdentifier]`
  geprueft werden.
- Eine Signatur ist noetig, ad hoc genuegt (`codesign --force -s -` ueber das
  ganze Bundle, nach dem Schreiben der Info.plist). Die Ad-hoc-Signatur des
  Go-Linkers allein deckt das Bundle nicht ab. Developer ID,
  Notarisierung und Hardened Runtime braucht nur die Weitergabe an Dritte
  (Gatekeeper). Time-sensitive und kritische Mitteilungen brauchen
  Entitlements und damit ein echtes Zertifikat.
- Ein in das Binary eingebettetes `__TEXT,__info_plist` hilft nicht: es
  braucht externes Linken (cgo) und ergibt keinen LaunchServices-Eintrag.
  `osascript` erscheint als Skripteditor, `NSUserNotification` ist
  veraltet und auf macOS 26 unzuverlaessig.
- Vorschlag fuer die Entwicklung: ein Wrapper fuer `go run -exec`, der das
  Binary in ein festes Bundle unter `~/Library/Application Support` legt,
  ad hoc signiert, registriert und direkt startet.

### Windows 11: umgesetzt und geprueft (01.11.2026)

- `notify/platform_windows.go`: WinRT-Toasts ueber syscall und
  handgeschriebene Vtables, alle Aufrufe auf einem Worker-Thread im MTA
  (`RoInitialize`). IIDs und Vtable-Reihenfolgen gegen die generierten
  Bindungen von microsoft/windows-rs geprueft. Tag pro ID in der Gruppe
  "gift", Ersetzen nativ ueber denselben Tag, Zurueckziehen ueber
  `History.RemoveGroupedTagWithId`. Vor jedem Show wird
  `ToastNotifier.Setting` gelesen (`ErrNotPermitted`).
- Klicks ueber COM statt ueber das Activated-Event: `CustomActivator` in der
  Registry, `CoRegisterClassObject` fuer eine aus der AUMID abgeleitete
  CLSID (UUID v5), statische IClassFactory und
  INotificationActivationCallback mit `windows.NewCallback`. Das deckt auch
  Klicks im Benachrichtigungscenter ab, solange die App laeuft; ohne
  `LocalServer32` tut ein Klick nach Programmende nichts. Beim Klick holt
  `SetForegroundWindow` das erste sichtbare Fenster des Prozesses nach vorn.
- `notify/registry_windows.go`: `DisplayName`, `CustomActivator`, `IconUri`
  (neues `Config.Icon`), nur geschrieben, wenn sie sich unterscheiden;
  `notify.Unregister` loescht den Schluessel. Ein Prozess mit Adminrechten
  bekommt `ErrUnsupported`.
- `notify/toast.go` ohne Build-Tag: Toast-XML mit Escaping, Entfernen von
  Steuerzeichen und Kuerzen auf 4 900 Bytes nach dem Escaping, Pruefung der
  AUMID, Ableitung der CLSID. Laeuft in jedem `go test`.
- `golang.org/x/sys` ist jetzt direkte Abhaengigkeit (war indirekt).
- Geprueft auf Windows 11 26H2 (Build 26300.9818, amd64, ohne
  Adminrechte): Mit dem Registry-Eintrag allein, ohne Verknuepfung, Paket
  oder Signatur, erscheint der Toast, und der Klick kommt ueber den
  COM-Activator in `OnClick` an. Die Berichte, 25H2 zeige so keine Banner
  mehr, treffen hier nicht zu; der Tray-Fallback bleibt zurueckgestellt.
- `ToastNotifier.Setting` ist fuer eine App ohne Paket nicht lesbar: der
  Aufruf scheitert mit `0x80070490` (ERROR_NOT_FOUND), obwohl der Toast
  erscheint. Der erste Lauf brach deshalb ab, bevor ein Toast gezeigt
  wurde. Jetzt gilt nur noch eine gelesene Sperre durch Benutzer oder
  Gruppenrichtlinie als `ErrNotPermitted`; ERROR_NOT_FOUND wird still
  hingenommen, andere Fehler und DisabledByManifest einmal geloggt. Folge:
  Hat der Benutzer die Toasts abgeschaltet, meldet gift unter Windows
  wahrscheinlich kein `ErrNotPermitted`, und `Authorize` antwortet `true`.
- Mit `example-notify.exe` ebenfalls bestaetigt: Ersetzen per Tag (nur ein
  Eintrag im Benachrichtigungscenter), `Withdraw`, und ein Klick holt das
  Fenster nach vorn, auch aus dem minimierten Zustand.

### Windows 11: Ergebnis der Recherche

WinRT-Toasts (`ToastNotificationManager.CreateToastNotifier(AUMID)`) ueber
syscall auf `combase.dll` und COM-Vtables von Hand. Weder MSIX noch eine
Signatur sind noetig; die Signatur wird erst fuer MSIX oder ein Sparse
Package Pflicht und betrifft beim nackten .exe nur SmartScreen. Noetig ist
eine AUMID, die Windows aufloesen kann:

- Eine nirgends registrierte AUMID wird seit Windows 10 1709 still
  verworfen: `Show` meldet Erfolg, angezeigt wird nichts.
  `SetCurrentProcessExplicitAppUserModelID` allein reicht nicht.
- Dokumentiert sind zwei Wege, beide pro Benutzer und ohne Adminrechte zur
  Laufzeit zu schreiben: der Registry-Schluessel
  `HKCU\Software\Classes\AppUserModelId\<AUMID>` (`DisplayName`, `IconUri`,
  `CustomActivator`; so machen es Windows App SDK, Community Toolkit und
  go-toast v2) oder eine Startmenue-Verknuepfung mit
  `System.AppUserModel.ID`. Zwei Berichte aus Sept./Okt. 2026 sehen auf
  Windows 11 25H2 (Build 26200) mit der Registry allein keine Banner mehr,
  mit Verknuepfung schon. gift sollte beides anlegen, abschaltbar, mit einer
  Funktion zum Entfernen.
- Der In-Process-Event `Activated` kommt ohne COM-Aktivator, solange die App
  laeuft und das Banner sichtbar ist. Klicks aus dem Benachrichtigungscenter
  und nach Programmende brauchen `INotificationActivationCallback` mit CLSID
  und `LocalServer32`.
- Ein erhoehter Prozess (Administrator) kann keine Toasts zeigen.
- Rueckfall ohne jede Registrierung: `Shell_NotifyIcon` mit `NIF_INFO`.
  Windows 11 zeigt daraus ein kurzes Banner ohne Eintrag im Center, unter
  dem `FileDescription` des Programms.
- Ob die Registry allein auf aktuellem 25H2 genuegt, muss auf einer echten
  Maschine geprueft werden, einmal mit und einmal ohne Verknuepfung.

### Entscheidungen fuer macOS und Windows (01.11.2026)

- macOS: Ein Show vor der Antwort auf die Berechtigungsfrage oder nach einem
  Nein wird mit `ErrNotPermitted` verworfen, nicht zurueckgehalten.
  `Notifier.Authorize` stellt die Frage zu einem Zeitpunkt, den die
  Anwendung waehlt; sonst stellt sie das erste Show.
- Windows: nur der Registry-Eintrag, nie eine Startmenue-Verknuepfung.
  Scheitert das auf 25H2, wird die Sprechblase am Tray-Icon vorgezogen.
- Windows: `Config.AppID` ist Pflicht und wird nicht abgeleitet.
- Windows wird auf einem echten PC des Nutzers geprueft; die Test- und
  Beispielprogramme werden auf dem Mac gebaut (`GOOS=windows`).
- Reihenfolge: gemeinsame Vorarbeit und macOS zuerst, dann Windows.

## Vorgemerkt: Fokusring als Halo ausserhalb

Zur spaeteren Diskussion (31.10.2026). Der Fokusring sitzt heute innerhalb
des Rahmens (`focusRingGap`), weil ein Ring ausserhalb vom ersten
Scroll-Container, einer Liste oder Tab-Leiste abgeschnitten wuerde. iPadOS
zeichnet den Fokus stattdessen als Halo *ausserhalb* der Ansicht in einer
Ebene darueber (UIFocusHaloEffect); macOS und das Web legen den Ring ebenfalls
nach aussen. Das wuerde die innere Luecke ueberfluessig machen, an der
gescrollter Text abgeschnitten werden muss (siehe Abschnitt 31 und 3db49cd).
Kandidat fuer den Mechanismus: die Top-Ebene (`Element.Overlay`) oder ein
eigener Malschritt ueber dem Inhalt. Betrifft jedes Control mit Ring.

## Quellen der Architekturpruefung

- Nago, untersuchter Stand: https://github.com/worldiety/nago/tree/76fb9e89b1169c9f6bc020a5b31971422898479c
- Nago View-Vertrag: https://github.com/worldiety/nago/blob/76fb9e89b1169c9f6bc020a5b31971422898479c/presentation/core/component.go
- Nago State: https://github.com/worldiety/nago/blob/76fb9e89b1169c9f6bc020a5b31971422898479c/presentation/core/state.go
- Go 1.27, generische Methoden, August 2026: https://go.dev/doc/go1.27
- Ebitengine 2.10/2.10.1: https://ebitengine.org/en/documents/2.10.html
- Ebitengine Desktop-Voraussetzungen: https://ebitengine.org/en/documents/install.html
- Image-Lifecycle und Upload-API: https://github.com/hajimehoshi/ebiten/blob/v2.10.1/image.go
- OpenGL-Upload und glFinish: https://github.com/hajimehoshi/ebiten/blob/v2.10.1/internal/graphicsdriver/opengl/image.go
- Update-/Draw-Vertrag: https://github.com/hajimehoshi/ebiten/blob/v2.10.1/run.go
- Kage-Shadersprache: https://ebitengine.org/en/documents/shader.html
- go-text/typesetting: https://github.com/go-text/typesetting

Die Machbarkeitsbewertung basiert auf Architektur- und Quellpruefung, nicht auf
bereits ausgefuehrten Pi-Benchmarks.
