# FileBrowser Quantum Mermaid Diagrams

This file contains a comprehensive set of Mermaid diagrams derived from the current repository structure and backend/frontend flow.

## 1) High-Level System Architecture

```mermaid
flowchart TB
  U[Browser Client<br/>Vue App] -->|HTTPS / API / SSE| H[Go HTTP Layer<br/>backend/http]

  subgraph B[Backend Runtime]
    H --> M[Middleware Stack<br/>auth, permissions, share guards]
    M --> RH[Route Handlers<br/>resources, auth, share, users, settings, tools, media]

    RH --> IDX[Indexing Engine<br/>backend/indexing]
    RH --> PRV[Preview Service<br/>backend/preview]
    RH --> EVT[Event Router<br/>backend/events]
    RH --> A[Auth Module<br/>backend/auth]

    PRV --> FFM[FFmpeg Service<br/>backend/ffmpeg]
  end

  subgraph S[Storage]
    DB[(Bolt DB<br/>users/settings/access/share)]
    SQL[(SQLite Index DB<br/>file indexes + metadata)]
    FS[(Source Filesystems<br/>configured source map)]
    CACHE[(Cache Dir<br/>thumbnails/temp)]
  end

  RH --> DB
  IDX --> SQL
  IDX --> FS
  PRV --> FS
  PRV --> CACHE
  A --> DB
  EVT --> H
```

## 2) Startup and Service Lifecycle

```mermaid
sequenceDiagram
  participant Main as main.go
  participant Cmd as cmd.StartFilebrowser
  participant Set as settings.Initialize
  participant Store as storage.InitializeDb
  participant Idx as indexing
  participant Prev as preview
  participant Http as http.StartHttp

  Main->>Cmd: StartFilebrowser()
  Cmd->>Set: Load config + env flags
  Cmd->>Store: Open/initialize Bolt storage
  Store-->>Cmd: store + dbExists

  Cmd->>Idx: InitializeIndexDB()
  Cmd->>Idx: SetIndexingStorage(store.Indexing)
  loop each configured source
    Cmd->>Idx: Initialize(source, ...)
  end

  Cmd->>Prev: StartPreviewGenerator(workers, cacheDir)
  Cmd->>Http: StartHttp(ctx, store, shutdownComplete)

  Note over Cmd,Idx: On shutdown: StopAllScanners() and close index DB
```

## 3) HTTP Router Map

```mermaid
flowchart LR
  Client[Client] --> Router[Main Router]

  Router --> API["/baseURL/api"]
  Router --> Public["/baseURL/public"]
  Router --> WebDAV["/baseURL/dav"]
  Router --> SPA[Index + Static + Swagger + Health]

  API --> Auth[auth/*<br/>login/logout/renew/token/oidc/otp]
  API --> Resources[resources/*<br/>CRUD/download/archive/preview]
  API --> Users[users]
  API --> Access[access/* + groups]
  API --> Share[share/*]
  API --> Settings[settings/*]
  API --> Tools[tools/search<br/>tools/fileWatcher<br/>tools/duplicateFinder]
  API --> Events[events SSE]
  API --> Media[media/subtitles]
  API --> Office[office/config + callback]

  Public --> PublicAPI[public /api subset via hash-share middleware]
```

## 4) Middleware and Auth Decision Flow

```mermaid
flowchart TD
  R[Incoming Request] --> X{Route Type}
  X -->|Protected API| U[withUser]
  X -->|Admin API| A[withAdmin -> withUser]
  X -->|Share/Public API| H[withHashFile or withOrWithoutUser]
  X -->|Open/health/static| O[no auth or minimal checks]

  U --> T[Extract token/cookie]
  T --> V{Valid user?}
  V -->|No| E1[401/403]
  V -->|Yes| P[Permission checks]
  P --> K{Allowed action?}
  K -->|No| E2[403]
  K -->|Yes| C[Handler execution]

  H --> S1[Resolve share hash]
  S1 --> S2[Validate share rules<br/>password/user allowlist/limits]
  S2 --> S3[Resolve source + path scope]
  S3 --> C
```

## 5) Login, Token Renewal, and API Token Lifecycle

```mermaid
sequenceDiagram
  participant FE as Frontend
  participant API as /api/auth/*
  participant AU as auth package
  participant US as Users Store
  participant AS as Access Store

  FE->>API: POST /auth/login
  API->>AU: Validate credentials + sign JWT
  AU-->>API: signed token
  API-->>FE: Set auth cookie

  FE->>API: POST /auth/renew
  API->>AU: Validate existing cookie token
  AU-->>API: refreshed signed token
  API-->>FE: Set renewed cookie

  FE->>API: POST /auth/token?name&days&permissions
  API->>AU: MakeSignedTokenAPI(...)
  AU-->>API: API token string + claims
  API->>US: AddApiToken(userID, name, token)
  API->>AS: AddApiToken(token, userID)
  API-->>FE: Return token

  FE->>API: DELETE /auth/token?name
  API->>US: DeleteApiToken(userID, name)
  API->>AU: RevokeApiToken(token)
  API->>AS: RemoveApiToken(token)
```

## 6) Indexing and Search Data Flow

```mermaid
flowchart TB
  subgraph Sources[Configured Sources]
    F1[Source A FS]
    F2[Source B FS]
  end

  Init[cmd.StartFilebrowser] --> IDB[indexing.InitializeIndexDB]
  Init --> Scans[indexing.Initialize per source]
  Scans --> Scanner[Scanners<br/>full/quick/routine scans]

  Scanner --> SQL[(SQLite Index DB)]
  Scanner --> Meta[(Persisted index metadata<br/>via indexing storage)]

  Query["/api/tools/search"] --> Prep[prepSearchOptions<br/>query/sources/scope/session]
  Prep --> Search{Single or Multi-source}
  Search -->|Single| S1[Index.Search]
  Search -->|Multi| S2[indexing.SearchMultiSources]

  S1 --> Filter[Access filter + scope trim]
  S2 --> Filter
  Filter --> Resp[SearchResult JSON]
```

## 7) Preview Generation Pipeline

```mermaid
sequenceDiagram
  participant C as Client
  participant H as previewHandler
  participant FI as FileInfoFaster
  participant P as preview.Service
  participant FC as File Cache
  participant FF as FFmpeg

  C->>H: GET /api/resources/preview?path&source&size
  H->>FI: Resolve file info + metadata
  FI-->>H: ExtendedFileInfo
  H->>P: GetPreviewForFile(ctx, file, size, ...)

  P->>FC: Load(cacheKey)
  alt Cache hit
    FC-->>P: image bytes
  else Cache miss
    P->>P: determinePreviewType(file)
    alt Document
      P->>P: GenerateImageFromDoc
    else Office
      P->>P: GenerateOfficePreview
    else HEIC/Video
      P->>FF: ffmpeg/ffprobe operations
      FF-->>P: image frame/converted bytes
    else Image/Audio art
      P->>P: image decode/resize or album-art reuse
    end
    P->>FC: Store(cacheKey, bytes)
  end

  P-->>H: preview image bytes
  H-->>C: image/jpeg response
```

## 8) Realtime Events and SSE

```mermaid
flowchart LR
  FE["Frontend SSE Client /api/events"] --> SSE["sseHandler"]
  SSE --> REG["events.Register"]
  REG --> UC["userClients"]
  REG --> SC["sourceClients"]

  Producers["System Producers indexing/share/office/misc"] --> EVT["events package"]
  EVT --> BQ["BroadcastChan"]
  EVT --> UQ["userEventChan"]
  EVT --> SQ["sourceUpdateChan"]

  BQ --> SSE
  UQ --> SSE
  SQ --> SSE
  SSE --> FE

  FE -->|disconnect| UNREG["events.Unregister"]
  UNREG --> UC
  UNREG --> SC
```

## 9) Share Link Lifecycle

```mermaid
stateDiagram-v2
  [*] --> Created: POST /api/share
  Created --> Active: hash generated + persisted

  Active --> Accessed: public/private request with hash
  Accessed --> Validated: password/token/user constraints pass
  Accessed --> Rejected: invalid hash/password/limit/permissions

  Validated --> Downloaded: file content fetched
  Downloaded --> Active: increment downloads (+ per-user counter)

  Active --> Updated: PATCH /api/share
  Updated --> Active

  Active --> Expired: expire timestamp reached
  Expired --> Active: keepAfterExpiration true
  Expired --> Deleted: cleanup/remove

  Active --> Deleted: DELETE /api/share
  Deleted --> [*]
```

## 10) Storage Model (Conceptual ER)

```mermaid
erDiagram
  USER ||--o{ API_TOKEN : owns
  USER ||--o{ SHARE_LINK : creates
  USER ||--o{ SOURCE_SCOPE : scoped_to

  SHARE_LINK }o--|| SOURCE : references
  INDEX_ENTRY }o--|| SOURCE : belongs_to
  ACCESS_RULE }o--|| SOURCE : applies_to

  USER {
    uint id
    string username
    string loginMethod
    string permissions
  }

  API_TOKEN {
    string name
    string token
    int64 expiresAt
    string permissions
  }

  SHARE_LINK {
    string hash
    uint userID
    string source
    string path
    int expire
    int downloads
    bool perUserDownloadLimit
  }

  SOURCE {
    string path
    string name
    bool private
  }

  SOURCE_SCOPE {
    string sourceName
    string scope
  }

  INDEX_ENTRY {
    string source
    string indexPath
    string type
    int64 size
    int64 modTime
  }

  ACCESS_RULE {
    string source
    string path
    string subject
    string rule
  }
```
