
> **Archived snapshot — not SoT.** See [README.md](./README.md).

# ProtoVoy 闃舵璁″垝涓庣己鍙ｅ鐓?

> 鍩轰簬 PRD V1.1銆佹妧鏈灦鏋?V1.0锛屼互鍙?2026-08-05 鍝佺墝涓庢妧鏈畾绋裤€?
> 瀵圭収鏃ユ湡锛?026-08-12锛堜唬鐮佸簱鐜扮姸蹇収锛夈€?

---

## 1. 瀹氱鍘熷垯锛堢浉瀵?PRD 鐨勮皟鏁达級

| 椤?| PRD 鍘熸枃鍊惧悜 | 椤圭洰瀹氱 |
|----|--------------|----------|
| 浜у搧鍚?| 鏈畾 | **ProtoVoy**锛堟棫绉?Reqonaut锛汚piLens 浠呬负鍐呴儴浠ｅ彿锛?|
| 璁稿彲璇?| 寰呭喅绛?| **Apache-2.0**锛堟湰浠撳簱鍏ㄩ儴婧愮爜锛夛紱鏈潵鎵樼浜戜笉鍦ㄦ湰浠撳簱 |
| MVP 鍗忎綔 | F-016 鏍囦负 P1 | **鍥㈤槦浜戝悓姝ユ斁 P2**锛汳VP 浠呮湰鍦板伐浣滃尯 + Git 鍙嬪ソ鏂囦欢 |
| 鑴氭湰寮曟搸 | F-009 鏍囦负 P0 | **P0 棰勭暀鐢熷懡鍛ㄦ湡**锛汸1 鎺ュ叆 QuickJS锛屼笉閲嶆瀯涓婚摼璺?|
| Desktop / Agent | 寰呭喅绛?| 鍏变韩 crates锛?*鍙屼簩杩涘埗**锛岀粺涓€鐗堟湰鎻℃墜 |
| 鎻掍欢 | WASM / 鍘熺敓 Sidecar 骞跺瓨 | 绗笁鏂?*浠?WASM**锛涘畼鏂瑰彲淇濈暀鍘熺敓閫傞厤鍣?|
| 鍗忎綔鍚庣 | Java 鍗忎綔鏈嶅姟 | **P2**锛欽ava 21 + Spring Boot 3 + Gradle Wrapper |

---

## 2. 鐜扮姸鎬昏

褰撳墠浠撳簱宸插畬鎴?**闃舵 0銆丮VP銆丳1 涓庝富瑕?P2 鏈湴/绉佹湁鍖栬兘鍔?*锛屾鍦ㄨ繘琛屽叕寮€ alpha 鍙戝竷楠岃瘉銆佺粏绮掑害鍥介檯鍖栦笌鍗忚绋冲畾鎬ф敹鍙ｃ€傜姸鎬佷互鍙獙璇佽瘉鎹负鍑嗭紝瑙佷笅鏂硅兘鍔涚煩闃碉紱鏈粡杩囩湡瀹炵敤鎴烽獙璇佺殑鑳藉姏涓嶆爣涓?Stable銆?

### 2.1 宸插叿澶囷紙鍙繍琛岄鏋讹級

| 鑳藉姏 | 璇佹嵁 / 璇存槑 |
|------|-------------|
| Monorepo + ProtoVoy 鍛藉悕 | `apps/`銆乣crates/`銆乣packages/`銆丄pache-2.0 |
| 缁熶竴鍩熸ā鍨?| `crates/core-domain`锛歚RequestEnvelope`銆乣ExecutionEvent` |
| 鎵ц寮曟搸 + 鍙栨秷 | `crates/execution-engine`锛欴river SPI銆乧ancel銆佸绾︽祴璇曢鏋?|
| 浜嬩欢娴侀€氶亾 | `crates/event-stream` |
| HTTP/HTTPS Driver | `crates/driver-http`锛坮eqwest锛屾祦寮?chunk銆佽秴鏃躲€佸彇娑堬級 |
| CLI 鐑熸祴 | `protovoy-cli http-get` / `drivers` |
| Local Agent | `127.0.0.1:39217`锛汢earer Token锛汷rigin 闄?`localhost:5180` |
| Desktop锛圱auri锛?| `http_get` / `cancel_execution` / `list_drivers` |
| Web 宸ヤ綔鍙?| 缁?Agent 璋冭瘯 HTTP锛屾敮鎸佹柟娉?Header/Body/璁よ瘉/鍙橀噺/鏂█/鍘嗗彶 |
| 鍏变韩 UI 澹?| `@protovoy/ui`锛歚AppShell` + `HttpWorkbench`锛孌esktop/Web 鍏辩敤 |
| TS 璇锋眰妯″瀷闀滃儚 | `@protovoy/request-model`锛堟墜鍐欙紝鍗忚 ID 绫诲瀷宸查鐣欙級 |
| 鐢熷懡鍛ㄦ湡鏋氫妇棰勭暀 | `LifecyclePhase` + `NoopLifecycleHook` |
| ADR | Tauri銆佸弻浜岃繘鍒躲€乄ASM-only 鎻掍欢 |

### 2.2 绔埌绔幇鐘讹紙鍞竴閫氳矾锛?

```text
HTTP 璇锋眰锛圙ET/POST/PUT/PATCH/DELETE/HEAD/OPTIONS锛?
  鈫?Desktop Tauri / Web鈫扐gent / CLI
  鈫?ExecutionEngine 鈫?HttpDriver 鈫?reqwest
  鈫?鎽樿 + 姝ｆ枃棰勮锛堚墹4KB锛?
```

### 2.3 鑳藉姏鐘舵€侊紙鍙獙璇侊級

| Capability | Status | Verification |
|------------|--------|--------------|
| HTTP execution | Beta | `crates/driver-http` integration tests + E2E smoke |
| GraphQL execution | Beta | `crates/driver-graphql` tests + HTTP GraphQL workbench |
| gRPC execution | Experimental | Frame/descriptor unit tests; reflection E2E pending |
| WebSocket / SSE / TCP / UDP | Beta | Driver tokio tests + workbench E2E layout |
| MQTT | Beta | `crates/driver-mqtt` connect/publish tests |
| AMQP / Kafka | Experimental | Validate-only unit tests; broker integration pending |
| Redis / SQL | Beta | Driver execution tests; no streaming |
| WASM plugins | Experimental | `crates/plugin-runtime` tests + SDK WIT |
| QuickJS scripts | Beta | Runtime limits + HTTP workbench |
| Collaboration / private deploy | Preview | Docker compose + Java service tests |
| Protocol gateway | Preview | `apps/protocol-gateway` + deploy docs |
| i18n | Beta | Core nav translated; workbench fine-grained strings incomplete |

---

## 3. 闇€姹傜己鍙ｇ煩闃?

鍥句緥锛氣渽 宸插疄鐜?路 馃煛 閮ㄥ垎/楠ㄦ灦 路 鉂?鏈紑濮?路 鈴?鎸夊畾绋垮欢鍚?

### 3.1 PRD 鍔熻兘闇€姹傦紙F-xxx锛?

| 缂栧彿 | 妯″潡 | PRD | 瀹氱闃舵 | 鐜扮姸 | 缂哄彛鎽樿 |
|------|------|-----|----------|------|----------|
| F-001 | 宸ヤ綔鍖轰笌椤圭洰 | P0 | P0 | 鉁?| 宸ヤ綔鍖?椤圭洰 CRUD銆佹悳绱€佸垏鎹€佸綊妗ｄ笌鎭㈠銆佹渶杩戣闂帓搴忓強 Web/Desktop 鍏变韩璧勬簮鏍戝潎宸插疄鐜?|
| F-002 | 闆嗗悎涓庣洰褰?| P0 | P0 | 鉁?| 宓屽闆嗗悎鏍戙€佹悳绱€丆RUD銆佹爣绛俱€佹嫋鎷藉綊绫讳笌鍚岀骇鎷栨嫿鎺掑簭銆佽姹傛壒閲忕Щ鍔?鍒犻櫎鍧囧凡瀹炵幇 |
| F-002A | 涓氬姟妯″潡涓庡崗璁祫婧?| P0 | P0 | 鉁?| 椤圭洰鍐呯疆榛樿妯″潡锛屾棫闆嗗悎鑷姩鍏煎杩佺Щ锛涜祫婧愭爲鎸夋ā鍧楃粍缁囨帴鍙ｏ紝骞舵寜宸蹭娇鐢ㄥ崗璁姩鎬佹樉绀虹被鍨嬪畾涔夈€丳roto/GraphQL Schema 涓庡叕鍏辩粍浠跺叆鍙ｏ紝涓嶄负鏃犳ā鍨嬪崗璁垱寤虹┖鐩綍 |
| F-003 | 缁熶竴璇锋眰缂栬緫鍣?| P0 | P0 | 鉁?| HTTP/SSE/TCP/UDP/GraphQL/WebSocket/gRPC 涓冨崗璁弻绔紪杈戝櫒銆佸彉閲忋€佽璇併€佹柇瑷€鍙婂崗璁笓灞為厤缃潎宸插疄鐜?|
| F-004 | 璇锋眰鎵ц | P0 | P0 | 鉁?| HTTP 鍙彂鍙彇娑堬紱閲嶈瘯/浠ｇ悊/TLS 鏍￠獙/閲嶅畾鍚戙€丆ookie Jar 鍙鍖栫鐞嗐€佹枃鏈?鏂囦欢 multipart銆並eychain 鍚堝苟 PEM 瀹㈡埛绔瘉涔﹀潎宸叉帴鍏?|
| F-005 | 鍝嶅簲宸ヤ綔鍙?| P0 | P0 | 鉁?| 鍒嗗潡鎺ユ敹銆?4MB 姝ｆ枃 Blob銆丠eaders銆丣SON 缇庡寲/鍘熸枃/Hex/琛ㄦ牸銆佸畬鏁存墽琛屼簨浠舵椂闂寸嚎銆佷竾瀛楃绐楀彛鍖栧ぇ鍝嶅簲娓叉煋銆佸鍒朵笌涓嬭浇鍧囧凡瀹炵幇 |
| F-006 | 鐜涓庡彉閲?| P0 | P0 | 鉁?| 澶氱骇浣滅敤鍩?+ `{{var}}`锛汥esktop/Agent 鐜鎸佷箙鍖?|
| F-007 | 鏈哄瘑绠＄悊 | P0 | P0 | 鉁?| Keychain + secret_ref锛涘鍑烘壂鎻忓睘 M3 |
| F-008 | 璁よ瘉 | P0 | P0 | 鉁?| Basic/Bearer/API Key銆丱Auth 2.0 Client Credentials銆丄uthorization Code + PKCE S256 娴忚鍣ㄦ巿鏉冿紙鎺堟潈鐮?verifier 浠呰繘鍏?Keychain/Agent secret-store锛夊強涓嶈惤鐩樼殑 TLS 瀹㈡埛绔瘉涔﹀潎宸插畬鎴?|
| F-009 | 鍓嶅悗缃剼鏈?| P0 | **P1** | 鉁?| QuickJS 鍓?鍚庣疆鑴氭湰銆乺equest/response/variables/console/assert/crypto API銆?6MB 鍐呭瓨涓?500ms 瓒呮椂闄愬埗銆丠TTP 鍙岀缂栬緫鍣ㄣ€佸彉閲忔彁鍙栦簨浠跺強 Agent/Desktop/CLI 璺ㄨ姹傛寔涔呭寲宸插疄鐜?|
| F-010 | 鏂█涓庢祴璇?| P0 | P0 | 鉁?| 鍐呯疆鐘舵€佺爜/鑰楁椂/澶у皬/Header/JSONPath/鏂囨湰鍖呭惈 |
| F-011 | 鍘嗗彶涓庨噸鏀?| P0 | P0 | 鉁?| 鏈湴鍘嗗彶銆佺瓫閫夈€佸姣斻€侀噸鏀撅紙Desktop + Agent/Web锛?|
| F-012 | 瀵煎叆瀵煎嚭 | P0 | P0 | 鉁?| cURL銆丱penAPI JSON/YAML銆丠AR銆丳ostman v2 涓?ProtoVoy 椤圭洰鍖呭凡鎺ュ叆锛涙敮鎸?OpenAPI 鍐呴儴鍙婅法鏂囦欢 `$ref`銆佸惊鐜娴嬨€丼chema 璇锋眰绀轰緥銆乻erver variables 涓?Postman variables 鏄犲皠锛屾枃浠跺す/tag/page 鏄犲皠瀛愰泦鍚堝強鏁忔劅鎵弿 |
| F-013 | 浠ｇ爜鐢熸垚 | P1 | P1 | 鉁?| HTTP 宸ヤ綔鍙版敮鎸?cURL銆丣avaScript Fetch銆丳ython Requests銆丟o net/http銆丷ust reqwest銆丣ava HttpClient锛汫raphQL/gRPC/WebSocket/SSE/TCP/UDP 鍧囨湁涓撶敤鐢熸垚鍣紝骞跺紑鏀?HTTP 涓庡崗璁ā鏉挎敞鍐?鍗歌浇鎻掍欢鐐?|
| F-014 | 闆嗗悎杩愯鍣?| P1 | P1 | 鉁?| CLI 鏀寔 RequestEnvelope/ProtoVoy 椤圭洰闆嗗悎銆侀『搴?骞跺彂銆佸け璐ュ嵆鍋溿€佽法璇锋眰鍙橀噺銆丣SON/CSV 鏁版嵁杩唬銆丣SON/JUnit 鎶ュ憡鍜岀粨鏋勫寲閫€鍑虹爜锛汥esktop 鏀寔褰撳墠闆嗗悎杩愯銆佸け璐ュ嵆鍋滀笌鏂█鎶ュ憡 |
| F-015 | Mock 鏈嶅姟 | P1 | P1 | 鉁?| 鐙珛 `protovoy-mock` 杩涚▼缁熶竴鎵胯浇鎵€鏈夐」鐩紝榛樿浠呭洖鐜洃鍚笖鏄惧紡鍚仠锛涙敮鎸侀」鐩?鏈嶅姟/鎺ュ彛闅旂銆佽矾寰勪笌 ID 涓ょ鍦板潃銆佽璁″搷搴旇嚜鍔ㄧ敓鏁堛€佽鍒欏惎鍋溿€佹潯浠跺尮閰嶃€丠TTP/WebSocket銆佸欢杩熴€佸懆鏈熼敊璇笌璇曡窇锛岃瑙?[MOCK_SERVICE.md](MOCK_SERVICE.md) |
| F-016 | 鍥㈤槦鍗忎綔 | P1 | **P2** | 鉁?| Java 21 / Spring Boot 3 鍗忎綔鏈嶅姟涓庡弻绔?Team/Comments/SSO 宸ヤ綔鍙板凡瑕嗙洊 Owner 寮曞銆侀偖绠卞瘑鐮佸強浼佷笟 OIDC 鐧诲綍銆佸凡楠岃瘉閭 JIT 鍏ョ粍銆佽仈鍚堣韩浠界粦瀹氥€佽澶囦細璇濄€佺粍缁囨垚鍛樸€佷簲绾?RBAC銆佽劚鏁忚祫婧愭爲蹇収銆佸箓绛夊眰绾ф仮澶嶃€乺evision/澧為噺鍚屾銆佸啿绐佸悎骞?UI銆佽瘎璁轰富棰?鍥炲/瑙ｅ喅鐘舵€併€丼SE 瀹炴椂鍙樻洿涓庡璁?|
| F-017 | 鎻掍欢涓績 | P1 | P1 | 鉁?| Wasmtime Component-only 瀹夸富銆乀ransformer/Protocol/Auth/Importer 涓撶敤 WIT 鍏ュ彛銆佹潈闄愭牎楠屻€丼HA-256銆侀槻绡℃敼銆丒d25519 鍙戝竷鑰呬俊浠婚摼銆佸唴瀛?fuel 闄愬埗銆丄gent/Desktop 瀹夎/鍚仠/鍗歌浇/鎸夌被鍨嬭皟鐢?API 涓?Web/Desktop 鎻掍欢涓績宸插疄鐜?|
| F-018 | AI 杈呭姪 | P2 | P2 | 鉁?| Web/Desktop AI 宸ヤ綔鍙版敮鎸佽嚜鐒惰瑷€鐢熸垚 HTTP 璇锋眰銆佸搷搴旇В閲娿€佹柇瑷€涓庢枃妗ｇ敓鎴愶紱OpenAI-compatible/Ollama BYOK锛屽瘑閽ヤ粎杩涘叆 Keychain/Agent secret-store锛岃繙绋?HTTPS 寮哄埗銆佺粨鏋滈瑙堝悗搴旂敤銆佸弻绔叡浜?Rust 瀹㈡埛绔笌 Provider 濂戠害娴嬭瘯鍧囧凡瀹屾垚 |
| F-019 | 娴侀噺浠ｇ悊鎶撳寘 | P2 | P2 | 鉁?| Web/Desktop Capture 宸ヤ綔鍙颁笌鍏变韩 Rust 鏄惧紡浠ｇ悊宸插畬鎴愶紱HTTP 璇锋眰/鍝嶅簲 Header 涓?64KB 姝ｆ枃棰勮銆丠TTPS CONNECT 鍔犲瘑闅ч亾鍏冩暟鎹€乴oopback 瀹夊叏榛樿銆佹晱鎰?Header 鑴辨晱銆?00 鏉＄幆褰繚鐣欍€佺敓鍛藉懆鏈熸帶鍒跺強涓€閿敓鎴?HTTP 璇锋眰鍧囧凡瀹炵幇 |
| F-020 | 绉佹湁鍖栭儴缃?| P2 | P2 | 鉁?| 鍗曞叆鍙?Nginx + Web + Rust Agent + Java 鍗忎綔/OIDC + PostgreSQL Compose 缂栨帓锛涜繍琛屾椂鍓嶇閰嶇疆銆佸唴閮ㄥ弽鍚戜唬鐞嗐€佹渶灏忕鍙ｆ毚闇层€佸仴搴锋鏌ャ€佹寔涔呭寲鍗枫€佺幆澧冩ā鏉裤€侀儴缃叉枃妗ｄ笌闀滃儚鏋勫缓 CI 宸插畬鎴?|

### 3.2 MVP 鍗忚鐭╅樀锛圙1 / AC-03锛?

| 鍗忚 | 鐜扮姸 | 缂哄彛 |
|------|------|------|
| HTTP/HTTPS | 鉁?Driver + 鍙岀宸ヤ綔鍙?| 缂栬緫鍣ㄣ€佽嚜鍔?Cookie Jar 涓庡弻绔鐞嗐€乵ultipart銆佷唬鐞?TLS/瀹㈡埛绔瘉涔?UI銆丱penAPI 瀵煎叆銆佷簨浠剁骇瀹炴椂鍝嶅簲 UI 涓庡搷搴旀寔涔呭寲宸插疄鐜?|
| WebSocket | 鉁?Driver + 鍙岀宸ヤ綔鍙?| ws/wss銆乀ext/Binary銆佸瓙鍗忚銆佽嚜瀹氫箟 Header銆丳ing/Pong銆佸彇娑堛€佽嚜鍔ㄩ噸杩炪€佸搷搴旀寔涔呭寲锛屼互鍙?Web/Desktop 鍘熺敓鎸佷箙杩炴帴鍚庣殑澶氭浜や簰鍙戦€佷笌鍙屽悜甯ф椂闂寸嚎鍧囧凡瀹炵幇 |
| SSE | 鉁?Driver + 鍙岀宸ヤ綔鍙?| 寮虹被鍨嬭浇鑽枫€佷簨浠剁骇澧為噺 UI銆佹祦寮忔帴鏀躲€佸彇娑堛€丩ast-Event-ID銆佹湇鍔＄ retry銆佸彲閰嶇疆鑷姩閲嶈繛銆佸彉閲?璁よ瘉鍜屽搷搴旀寔涔呭寲宸插疄鐜?|
| GraphQL | 鉁?Driver + 鍙岀宸ヤ綔鍙?| Query/Mutation/WS Subscription銆乂ariables銆乷perationName銆佽嚜瀹氫箟 Header銆丼chema Introspection/绫诲瀷娴忚銆丮onaco 璇硶楂樹寒涓庡叧閿瓧琛ュ叏銆丟raphQL errors 鎻愮ず銆佸彇娑堜笌鍘嗗彶鎸佷箙鍖栧潎宸插疄鐜?|
| gRPC | 鉁?Driver + 鍙岀宸ヤ綔鍙?| 閫氱敤 protobuf Base64銆丠TTP/2銆乁nary/Server/Client/Bidi 鍥涚被娴併€丮etadata銆丗ileDescriptorSet 瀵煎叆銆丼erver Reflection 鑷姩鎻忚堪绗﹀彂鐜般€丣SON鈫擯rotobuf銆佸彇娑堜笌鍝嶅簲鎸佷箙鍖栧潎宸插疄鐜?|
| TCP | 鉁?Driver + 鍙岀宸ヤ綔鍙?| 鏂囨湰/Hex銆佸浐瀹氶暱搴?鍒嗛殧绗﹀垎甯с€佺郴缁熸牴/鑷畾涔?CA TLS銆丼NI銆佸畾鏃堕噸澶嶅彂閫併€佽秴鏃躲€佸彇娑堛€佸搷搴旀寔涔呭寲锛屼互鍙婄粡閰嶅 Token 淇濇姢鐨?Agent 鎸佷箙 TCP 浜や簰妗ユ帴鍧囧凡瀹炵幇 |
| UDP | 鉁?Driver + 鍙岀宸ヤ綔鍙?| 鏂囨湰/Hex銆侀噸澶?瀹氭椂鍙戦€併€佹帴鏀惰秴鏃躲€佸彇娑堝拰鍝嶅簲鎸佷箙鍖栧凡瀹炵幇 |

> P1 鍗忚鎵╁睍宸插畬鎴?SOAP銆丣SON-RPC銆丷edis銆丮QTT 3.1.1銆丄MQP銆並afka 涓?PostgreSQL/MySQL/SQLite锛屽潎宸叉敞鍐屽埌 Agent/Desktop/CLI 骞舵彁渚?Web/Desktop 宸ヤ綔鍙般€?

### 3.3 鏋舵瀯鏂囨。妯″潡缂哄彛

| 妯″潡锛堟灦鏋?搂5 / 搂8鈥?2锛?| 鐜扮姸 | 鐩爣闃舵 |
|-------------------------|------|----------|
| SQLite 鏈湴搴擄紙workspace/request/history鈥︼級 | 鉁?| P0-M0/M1 |
| Agent 姝ｅ紡 API锛坋xecutions + SSE + env/history/request锛?| 鉁?| 鎵ц闂幆銆侀暱鏈熼厤瀵?Token 鈫?8 灏忔椂 session Token 浜ゆ崲銆佽繃鏈熸竻鐞嗗強 TCP 浼氳瘽妗ユ帴閴存潈鍧囧凡瀹炵幇 |
| Desktop 鈫?Agent 鐗堟湰鎻℃墜 | 鉁?`protocol_api_version: "1"` | P0-M0 |
| Desktop 鎹嗙粦 Agent sidecar | 鉁?| 鏋勫缓閽╁瓙鎸?Rust host triple 鑷姩缂栬瘧銆侀噸鍛藉悕骞堕€氳繃 Tauri `externalBin` 鎹嗙粦 `protovoy-agent` |
| OS Keychain 鎺ュ叆 | 鉁?| P0-M1 |
| 鍙橀噺瑙ｆ瀽 + 鍐呯疆鏂█ | 鉁?| P0-M1 |
| 瀵煎叆瀵煎嚭鍖?`packages/import-export` | 鉁?| 宸插疄鐜?JSON/YAML 鏍煎紡璇嗗埆銆佸洓绫诲鍏ャ€佸唴閮?澶栭儴 `$ref`銆丼chema 绀轰緥銆佺幆澧冨彉閲忔槧灏勩€佸眰绾ф槧灏勩€侀」鐩寘瀵煎嚭涓庢晱鎰熸壂鎻?|
| Monaco / 鍛戒护闈㈡澘 / i18n | 馃煛 | 绂荤嚎鏈湴 Monaco 宸茬敤浜?HTTP Body銆丵uickJS銆丟raphQL/JSON锛汣trl/鈱?K銆佸崗璁爣绛惧伐浣滃彴銆佽瑷€璧勬簮杩愯鏃躲€佷腑鏂?鑻辨枃鎸佷箙鍖栧垏鎹€佹牳蹇冨鑸強鍏ㄩ儴宸ヤ綔鍙版爣绛剧炕璇戝凡瀹炵幇锛涘緟杩佺Щ宸ヤ綔鍙板唴閮ㄧ粏绮掑害鏂囨 |
| TanStack Query + Zustand 鍒嗗眰 | 鉁?| Web/Desktop 鍏变韩 QueryClient Provider锛汫ateway 鏈嶅姟绔换鍔?鍘嗗彶浣跨敤 TanStack Query 缂撳瓨锛屾椿鍔ㄥ伐浣滃彴绛夊鎴风鐘舵€佷娇鐢?Zustand 鎸佷箙鍖?Store |
| `plugin-runtime` + Wasmtime | 鉁?| Component-only 鍥涚被鎻掍欢瀹夸富銆佹潈闄愪笌璧勬簮闄愬埗銆丒d25519 绛惧悕鏍￠獙銆佸弻绔彃浠朵腑蹇冨凡鎺ュ叆 |
| QuickJS 鑴氭湰 | 鉁?| 鍙楅檺杩愯鏃躲€乺equest/response/variables/console/assert/crypto API銆丠TTP 鍙岀缂栬緫鍣ㄥ拰璺ㄨ姹傚彉閲忔暟鎹摼宸叉帴鍏?|
| Java 鍗忎綔寰湇鍔℃媶鍒?| 馃煛 | 鍙儴缃插崟浣撳凡瑕嗙洊 identity/OIDC SSO/workspace/sync/audit锛屾敮鎸?H2 鏈湴涓?PostgreSQL/Docker 绉佹湁鍖栵紱mock/automation 鎷嗗垎寰呭悗缁垏鐗?|
| 浜戝崗璁綉鍏?`protocol-gateway` | 鉁?| 鐙珛 Rust 鏈嶅姟澶嶇敤鍏ㄩ儴鍗忚 Driver锛屾彁渚?Bearer 閴存潈杩滅▼鎵ц銆佹寔涔呭寲闂撮殧璋冨害銆丆I Runner 缁撴瀯鍖栭€€鍑虹爜銆佸苟鍙戜笂闄愩€佹墽琛岀暀瀛樹笌鏄庣‘鏁版嵁娴佸悜 |
| CI锛氫笁骞冲彴鏋勫缓 / E2E / SBOM | 鉁?| GitHub Actions 宸茶鐩栦笁骞冲彴 Desktop/sidecar 鏋勫缓銆丩inux DEB/Windows MSI/macOS App 瀹夎涓庡嵏杞姐€丆LI/Agent 涓存椂瀹夎鍚姩涓庢竻鐞嗐€丳laywright 妗岄潰/绉诲姩娴忚鍣ㄥ啋鐑熴€佸叏宸ヤ綔鍖烘祴璇曘€丆lippy銆丷ustSec銆丆ycloneDX SBOM銆佺鏈夊寲闀滃儚鏋勫缓涓?tag 鑽夌鍙戝竷 |

### 3.4 MVP 楠屾敹鏍囧噯锛圓C锛夊鐓?

| 缂栧彿 | 楠屾敹鏉′欢 | 鐜扮姸 |
|------|----------|------|
| AC-01 | 涓夊钩鍙板畨瑁?鍗囩骇/鍗歌浇涓€鑷?| 鉁?涓夊钩鍙?Tauri + Agent/CLI/Gateway 鍙戝竷娴佹按绾裤€佸畬鏁村钩鍙板浘鏍囧強 Linux DEB銆乄indows MSI銆乵acOS App 瀹夎/楠岃瘉/鍗歌浇 CI 宸插缓绔?|
| AC-02 | Web 瀹屾垚 HTTP/GraphQL/WS + 瀹夊叏杩?Agent | 鉁?HTTP/GraphQL/WS 姝ｅ紡鎵ц閫氳矾涓庨厤瀵?Bearer Token 宸插畬鎴?|
| AC-03 | 涓冨崗璁垱寤?淇濆瓨/鍙戦€?鍙栨秷/鍝嶅簲/鍘嗗彶 | 鉁?Web/Desktop 涓冨崗璁繚瀛樸€佽祫婧愭爲鎸夊崗璁仮澶嶃€佸彂閫?鍙栨秷銆佸搷搴旀寔涔呭寲涓庣粺涓€鍘嗗彶鍥炴斁鍧囧凡璐€?|
| AC-04 | 鍙橀噺銆丼ecret銆佽璇併€佽剼鏈€佹柇瑷€澶嶇敤 | 鉁?鍙橀噺/Secret/浜旂被璁よ瘉/QuickJS 鍓嶅悗缃剼鏈?鏂█宸茶疮閫?|
| AC-05 | OpenAPI/cURL/HAR/Postman + 鏁忔劅妫€娴?| 鉁?鍥涚被瀵煎叆涓?Header/Query/Body 鏁忔劅妫€娴嬪凡瀹屾垚 |
| AC-06 | gRPC 鍥涚被娴侊紱TCP/UDP 鏂囨湰+Hex锛沇S 鍙屽抚 | 鉁?gRPC 鍥涚被娴佹壒娆?閫愬抚缂栬В鐮併€乀CP/UDP Text+Hex銆乄S Text/Binary 鍧囧凡瀹炵幇 |
| AC-07 | 10MB/闀挎祦涓嶅喕 UI锛涘彲鍙栨秷 | 鉁?鍐呮牳鍙彇娑堛€佹鏂囧缃?Blob銆佷簨浠跺閲忔帴鏀朵笌涓囧瓧绗︾獥鍙ｆ覆鏌撳凡瀹屾垚 |
| AC-08 | Secret 涓嶆槑鏂囪惤鐩?鏃ュ織/瀵煎嚭 | 鉁?Keychain/Agent secret-store + runtime secret + 瀵煎嚭鏁忔劅鎵弿宸插畬鎴?|
| AC-09 | 寮傚父閫€鍑烘暟鎹畬鏁?+ 鑽夌鎭㈠ | 鉁?璇锋眰/鐜/鍘嗗彶宸叉寔涔呭寲锛汬TTP銆丟raphQL銆乬RPC銆乄ebSocket銆丼SE銆乀CP/UDP 宸ヤ綔鍙拌崏绋垮潎鑷姩淇濆瓨骞跺湪閲嶅惎鍚庢仮澶嶏紝鎹熷潖鑽夌鑷姩涓㈠純 |
| AC-10 | 鍗曟祴/闆嗘垚/E2E/瀹夊叏/瀹夎鍐掔儫 | 鉁?Rust/TS 鍗曟祴銆丳laywright 妗岄潰/绉诲姩娴忚鍣?UI 鍐掔儫銆佷笁骞冲彴瀹夎鐢熷懡鍛ㄦ湡銆丆lippy銆丷ustSec 涓?SBOM 鍧囧凡绾冲叆 CI |

---

## 4. 鍒嗘湡鎬昏

```text
闃舵 0锛堝綋鍓嶏級     M0 鏋舵瀯鏀跺彛
       鈫?
MVP / P0           M1 鍐呮牳 鈫?M2 涓冨崗璁?鈫?M3 鑷姩鍖栦笌瀵煎叆 鈫?M4 宸ヤ綔鍙版墦纾?
       鈫?
P1                 鑴氭湰 / WASM 鎻掍欢 / MQ路DB / Mock / CLI 闆嗗悎 / 瀵煎叆澧炲己
       鈫?
P2锛堝彲閫?OSS 绉佹湁鍖?/ Preview锛? 韬唤 / 鍥㈤槦鍚屾 / 瀹¤ / Java 鍗忎綔鏈嶅姟 / 鍗忚缃戝叧 / 绉佹湁鍖?
```

| 闃舵 | 寤鸿鍛ㄦ湡 | 閫€鍑烘潯浠讹紙涓€鍙ヨ瘽锛?|
|------|----------|--------------------|
| **闃舵 0** | 1鈥? 鍛紙鏀跺彛锛?| Desktop / Web+Agent / CLI 鍏辩敤姝ｅ紡鎵ц API 鍙?HTTP锛汼QLite 鍙瓨璇锋眰 |
| **MVP (P0)** | 12鈥?6 鍛?| 婊¤冻瀹氱鍚庣殑 MVP 楠屾敹锛堜竷鍗忚 + 鏈湴宸ヤ綔鍖?+ 鍙橀噺/璁よ瘉/鏂█/鍘嗗彶/瀵煎叆锛?|
| **P1** | 12鈥?8 鍛?| QuickJS銆乄ASM 鎻掍欢銆丮Q/DB銆丮ock銆侀泦鍚堣繍琛屽櫒鍙敤 |
| **P2** | 鎸佺画 | 鍥㈤槦鍚屾涓庝紒涓氱鏈夊寲鍙儴缃?|

---

## 5. 闃舵 0 鏀跺彛锛堢珛鍗虫墽琛岋級

鐩爣锛氭妸銆岀儫娴嬮鏋躲€嶅彉鎴愩€屽彲杩唬浜у搧鍩虹嚎銆嶃€?

| # | 鍒囩墖 | 浜や粯鐗?| 閫€鍑烘爣鍑?|
|---|------|--------|----------|
| 0.1 | 鍝佺墝娈嬬暀娓呯悊 | UI 鍘?鈥淩q鈥?Reqonaut锛涚粺涓€ ProtoVoy 鏂囨 | 鐢ㄦ埛鍙闈㈡棤鏃у悕 |
| 0.2 | 姝ｅ紡鎵ц API | Agent锛歚POST /v1/executions` + `GET .../events`锛圫SE锛? cancel锛汥esktop 鍚岀瓑 Command | 涓嶅啀渚濊禆 `/v1/debug/http-get` 浣滀负涓昏矾寰?|
| 0.3 | 鐗堟湰鎻℃墜 | `desktopVersion` / `agentVersion` / `protocolApiVersion` | 涓嶅吋瀹规椂鎷掔粷骞舵彁绀哄崌绾?|
| 0.4 | SQLite 鏈€灏忓簱 | project / collection / request / execution 鎽樿琛?+ 杩佺Щ | 淇濆瓨/鎵撳紑涓€涓?HTTP 璇锋眰 |
| 0.5 | HTTP 缂栬緫鍣ㄦ渶灏忛泦 | method銆乭eaders銆乥ody銆乼imeout | Desktop 涓?Web 琛屼负涓€鑷?|
| 0.6 | 楠屾敹鐑熸祴 | CLI + Desktop + Web+Agent 鍚勮窇閫氫竴娆?| 鏂囨。楠屾敹娓呭崟鍙嬀閫?|

**闃舵 0 楠屾敹閿氱偣**

- [x] Monorepo 浣跨敤 ProtoVoy 鍛藉悕涓?Apache-2.0
- [x] `LifecyclePhase` 閽╁瓙棰勭暀
- [x] Agent 鏈満鐩戝惉 + Bearer/Origin 鍩虹闃叉姢
- [x] `protovoy-cli http-get` 绾冲叆鍙噸澶嶉獙璇侊紙鏂囨。/鑴氭湰锛夆€?瑙?[`SMOKE_CHECKLIST.md`](./SMOKE_CHECKLIST.md)
- [x] Desktop / Web+Agent 缁?*姝ｅ紡鎵ц API**瀹屾垚 HTTP 閫氳矾锛坄POST /v1/executions` + SSE锛汥esktop `execute_request`锛?
- [x] 鏈湴 SQLite 鍙繚瀛樺苟閲嶆柊鎵撳紑璇锋眰锛坄crates/local-store` + Desktop 淇濆瓨/鎵撳紑锛?
- [x] Desktop 鈫?Agent 鐗堟湰鎻℃墜瀛楁鐢熸晥锛坄protocolApiVersion` / `agentVersion` / `desktopVersion`锛涗笉鍏煎 426锛?

---

## 6. MVP / P0 璇︾粏璁″垝

浠峰€奸摼锛堝繀椤绘墦閫氾級锛?

```text
閫夊崗璁?鈫?缂栬緫璇锋眰 鈫?閫夌幆澧?鍙橀噺 鈫?Desktop 鎴?Agent 鍙戦€?
鈫?娴佸紡鍝嶅簲 鈫?鏂█ 鈫?鍐欏叆鍘嗗彶 鈫?淇濆瓨鍒伴泦鍚?鈫?鍐嶆鎵ц
```

鏁版嵁榛樿锛?*SQLite + 鏈湴 Blob 鏂囦欢 + OS Keychain**锛涗笉寮哄埗娉ㄥ唽銆佷笉渚濊禆鏈嶅姟鍣ㄣ€?

### 6.1 M1 鈥?鍐呮牳涓庢湰鍦版暟鎹紙绾?3鈥? 鍛級

| 宸ヤ綔鍖?| 鍐呭 | 杩涘害 |
|--------|------|------|
| 鎸佷箙鍖?| SQLite 琛細workspace/project/collection/request/environment/variable/secret_ref/execution/blob_index锛涘ぇ姝ｆ枃澶栫疆 | 鉁?`workspace` + `blob_index`锛涜姹傚ぇ姝ｆ枃 / 澶ч瑙堝缃紙鈮?4KB锛夛紱variable/secret_ref 浠嶅祵鍦?environment JSON锛圡1 鍙帴鍙楋級 |
| 鍙橀噺 | 澶氱骇浣滅敤鍩?+ `{{var}}` / 鍔ㄦ€佸彉閲忥紱瑙ｆ瀽闃舵鎸傜敓鍛藉懆鏈?| 鉁?`VariableScope` + `{{var}}`/`$uuid`/`$timestamp`/`$isoTimestamp` 宸叉帴鍏ュ紩鎿?|
| Secret | Keychain 璇诲啓锛涢」鐩粎瀛?`secret_ref`锛涗簨浠?瀵煎嚭鑴辨晱绠￠亾楠ㄦ灦 | 鉁?Keychain + UI銆屽瓨鍏ュ瘑閽ャ€? Agent `PUT /v1/secrets`锛涘鍑烘晱鎰熸壂鎻忓睘 M3锛屾湰閲岀▼纰戜笉瑕佹眰 |
| 璁よ瘉 | Basic銆丅earer銆丄PI Key锛涜瘉涔﹀紩鐢紙TLS 瀹㈡埛绔瘉涔﹀彲鍚庣疆鍗婂懆锛?| 鉁?Basic/Bearer/API Key 寮曟搸娉ㄥ叆 + Desktop/Web UI锛汿LS 瀹㈡埛绔瘉涔︽寜璁″垝鍚庣疆 |
| 鏂█锛堟棤鑴氭湰锛?| 鐘舵€佺爜銆佽€楁椂銆佸ぇ灏忋€丠eader銆丣SONPath銆佹枃鏈寘鍚?| 鉁?寮曟搸鍐呯疆鏂█ + Desktop/Web UI 鏂囨湰 DSL |
| 鍘嗗彶 | 鏈湴鎵ц鎽樿銆佺瓫閫夈€侀噸鏀俱€佷粠鍘嗗彶鐢熸垚璇锋眰 | 鉁?Desktop/Agent/Web 鍧囨寔涔呭寲锛涚瓫閫夛紙state/status锛? 鍙屾潯棰勮瀵规瘮 + 閲嶆斁 |
| 閿欒妯″瀷 | Validation / Resolution / Connection / TLS / Protocol 缁熶竴瀛楁 | 鉁?`ErrorKind` 杩佸叆 `core-domain`锛沗DomainError` 瑕嗙洊鍏?kind锛涘紩鎿?`to_domain()` / wire code 瀵归綈 |

**閫€鍑?*锛欻TTP 璇锋眰鍙€屼繚瀛?鈫?鎹㈢幆澧冨彉閲?鈫?鍙戦€?鈫?鏂█ 鈫?鍘嗗彶閲嶆斁銆嶉棴鐜€傗渽锛圖esktop 涓?Web鈫扐gent 鍧囧凡鎵撻€氾級

**M1 鏄庣‘鍚庣疆锛堥潪鏈噷绋嬬闃诲锛?*

- TLS 瀹㈡埛绔瘉涔︼紙璁″垝鍐欐槑鍙悗缃級
- OAuth 鎺堟潈鐮佹祦锛埪?1锛歅0 鏈垨 P1锛?
- 瀵煎嚭鏁忔劅鎵弿锛圡3 宸ヤ綔鍖咃級

### 6.2 M2 鈥?MVP 涓冨崗璁紙绾?5鈥? 鍛級

| 椤哄簭寤鸿 | Crate / 鑳藉姏 | 濂戠害娴嬭瘯鏈€浣庨泦 |
|----------|--------------|----------------|
| 1 | 澧炲己 `driver-http`锛圕ookie銆佷唬鐞嗐€侀噸瀹氬悜绛栫暐銆佹祦寮?UI锛?| 鎴愬姛/瓒呮椂/鍙栨秷/TLS/澶у搷搴?|
| 2 | `driver-sse` | 浜嬩欢绫诲瀷銆侀噸杩炪€佸彇娑?|
| 3 | `driver-websocket` | 鏂囨湰/浜岃繘鍒躲€丳ing銆佹柇寮€銆佸彇娑?|
| 4 | GraphQL锛圚TTP + WS 璁㈤槄閫傞厤锛?| Query/Mutation锛涜闃呭悗缃彲闄嶇骇涓?P1 鑻ユ帓鏈熺揣 |
| 5 | `driver-grpc`锛坱onic锛?| Unary 浼樺厛锛涘啀琛?Client/Server/Bidi |
| 6 | `driver-tcp-udp` + 鍒嗗抚鍣?| 鏂囨湰/Hex锛汿CP TLS锛涘浐瀹氶暱/鍒嗛殧绗﹀垎甯?|

鍚屾鍓嶇锛?

- `Protocol Editor Host`锛堟寜 Driver Descriptor / UI Schema 鍒囨崲锛?
- 浜嬩欢娴佹帶鍒跺彴锛堝閲忔覆鏌撱€佹殏鍋滄覆鏌撱€佸彇娑堬級
- 闀胯繛鎺ワ細杩炴帴/鏂紑/閲嶈繛/娑堟伅鏃堕棿绾?

**閫€鍑?*锛氫竷鍗忚鍧囧彲鍒涘缓銆佷繚瀛樸€佸彂閫併€佸彇娑堛€佹煡鐪嬩簨浠?鍝嶅簲锛沢RPC 鑷冲皯 Unary + 涓€绉?Streaming锛汿CP/UDP 鏂囨湰+Hex銆?

### 6.3 M3 鈥?瀵煎叆瀵煎嚭涓庤嚜鍔ㄥ寲鍩虹嚎锛堢害 3鈥? 鍛級

| 宸ヤ綔鍖?| 鍐呭 |
|--------|------|
| 瀵煎叆 | OpenAPI 3銆乧URL銆丠AR銆丳ostman Collection v2 |
| 瀵煎嚭 | 椤圭洰鍖?/ 闆嗗悎锛涘鍑哄墠鏁忔劅瀛楁鎵弿 |
| 鏂囦欢鏍煎紡 | `*.protovoy.json`锛圙it 鍙嬪ソ锛変笌 SQLite 骞跺瓨绛栫暐钀藉湴锛堝洖搴?ADR-008锛?|
| CLI | 杩愯鍗曚釜璇锋眰 / 绠€鍗曢泦鍚堬紱缁撴瀯鍖栭€€鍑虹爜 |
| 鍝嶅簲瑙嗗浘 | JSON/XML 鏍煎紡鍖栥€丠ex銆佽櫄鎷熷垪琛紙10MB 鍦烘櫙锛?|
| 鑽夌鎭㈠ | 寮傚父閫€鍑烘湭淇濆瓨鑽夌鍙仮澶?|

**閫€鍑?*锛欰C-05 / AC-07 / AC-09 鍙紨绀猴紱GUI 涓?CLI 瀵瑰悓涓€璇锋眰缁撴灉涓€鑷淬€?

### 6.4 M4 鈥?宸ヤ綔鍙颁笌鍙戝竷鎵撶（锛堢害 2鈥? 鍛級

| 宸ヤ綔鍖?| 鍐呭 |
|--------|------|
| 淇℃伅鏋舵瀯 | 宸︼細椤圭洰/闆嗗悎/鍘嗗彶锛涗腑锛氱紪杈戝櫒锛涘簳锛氬搷搴?浜嬩欢/鏂█锛涘彸锛氱幆澧?|
| UX | 鍛戒护闈㈡澘銆佸揩鎹烽敭銆佺幆澧冪敓浜ф爣绛捐鍛?|
| Agent UX | 閰嶅鐮佹祦绋嬶紙鏇夸唬绾幆澧冨彉閲忔嫹璐濓級锛涜兘鍔涘垪琛ㄥ睍绀?|
| 璐ㄩ噺 | 鍗忚濂戠害娴嬭瘯鐭╅樀锛汥esktop/Web 鍩虹 E2E锛涗笁骞冲彴瀹夎鍖呭啋鐑?|
| 瀹夊叏 | Origin/浠ょ墝瀹屽杽锛汼ecret 涓嶆槑鏂囷紱鐢熶骇鍐欐搷浣滀簩娆＄‘璁わ紙涓汉妯″紡锛?|

**閫€鍑?*锛氬唴閮ㄥ洟闃熷彲鐢?ProtoVoy 鏇夸唬鏃ュ父 HTTP + 鑷冲皯 2 绉嶉潪 HTTP 鍗忚璋冭瘯銆?

### 6.5 P0 鏄庣‘涓嶅仛锛堥伩鍏嶈寖鍥磋啫鑳€锛?

- 鍥㈤槦浜戝悓姝ャ€丷BAC銆佽瘎璁恒€佸璁℃湇鍔?
- QuickJS 鐢ㄦ埛鑴氭湰锛堜粎淇濈暀閽╁瓙涓庡唴缃柇瑷€锛?
- WASM 绗笁鏂规彃浠跺競鍦?
- MQTT/Kafka/Redis/DB/Mock
- AI銆佸叏娴侀噺鎶撳寘銆佷簯鍗忚缃戝叧

---

## 7. P1 璁″垝锛圡VP 涔嬪悗锛?

| 涓婚 | 浜や粯 | 渚濊禆 |
|------|------|------|
| QuickJS 鑴氭湰 | 鎸傝浇鍚屼竴鐢熷懡鍛ㄦ湡锛涘彈鎺?API锛坮equest/response/variables/crypto/assert锛?| M1 鐢熷懡鍛ㄦ湡 |
| WASM 鎻掍欢 | 鉁?Wasmtime + 鍥涚被涓撶敤 WIT銆佹潈闄愭ā鍨嬨€丒d25519 淇′换閾句笌鎻掍欢涓績鍧囧凡瀹炵幇 | ADR-0003 |
| 鍗忚鎵╁睍 | 鉁?SOAP銆丣SON-RPC銆丷edis銆丮QTT銆丄MQP銆並afka銆丼QL 宸插畬鎴?| MQTT 瑕嗙洊 MQTTS銆佽嚜瀹氫箟 CA/SNI銆丵oS 0/1/2 鍙屽悜鐘舵€佹満锛汼QL 瑕嗙洊 PostgreSQL/MySQL/SQLite銆佸弬鏁般€佷簨鍔′笌缁撴灉闄愰 |
| Mock | 鉁?HTTP/WebSocket Mock銆佽鍒欐寔涔呭寲銆佺‘瀹氭€у尮閰嶃€佸欢杩熶笌鍛ㄦ湡閿欒娉ㄥ叆銆乄eb/Desktop 绠＄悊鍙板潎宸插畬鎴?| 鏈湴鎵ц灞?|
| 闆嗗悎杩愯鍣?| 鉁?CLI 鏀寔 RequestEnvelope/ProtoVoy 椤圭洰 JSON銆侀『搴?骞跺彂銆佸け璐ュ嵆鍋溿€佽法璇锋眰鍙橀噺銆丣SON/CSV 鏁版嵁杩唬鍙?JSON/JUnit 鎶ュ憡锛汥esktop 鎻愪緵闆嗗悎鎶ュ憡椤?| CLI + 鏂█ |
| 浠ｇ爜鐢熸垚 | 澶氳瑷€鐗囨 + 妯℃澘鎻掍欢鐐?| 璇锋眰妯″瀷绋冲畾 |
| Desktop sidecar | 鍙€夋崋缁?`protovoy-agent` | 鐗堟湰鎻℃墜 |

**閫€鍑?*锛氱涓夋柟鍙寜 SDK 寮€鍙戝苟瀹夎涓€涓?WASM 鍗忚鎻掍欢锛汣LI 鍙湪 CI 璺戦泦鍚堛€?

---

## 8. P2 璁″垝锛堝彲閫?OSS 绉佹湁鍖?/ Preview锛?

| 涓婚 | 浜や粯 |
|------|------|
| 韬唤涓庣粍缁?| 鐧诲綍銆丼SO銆佽澶囦細璇?|
| 宸ヤ綔鍖哄悓姝?| 澧為噺鍙樻洿銆乺evision銆佸啿绐?UI锛汼ecret 浠呭悓姝ュ紩鐢?|
| RBAC / 瀹¤ | Owner/Admin/Editor/Runner/Viewer锛涢珮椋庨櫓鎿嶄綔瀹¤ |
| Java 鏈嶅姟 | identity / workspace / sync / audit / mock / automation锛堝彲鍗曚綋璧锋锛?|
| 浜戝崗璁綉鍏?| 杩滅▼鎵ц銆佸畾鏃躲€丆I Runner锛涙暟鎹祦鍚戞槑绀?|
| 浼佷笟 | 绉佹湁鍖栭儴缃层€佺粍缁囩瓥鐣ャ€佹彃浠剁鍚嶇瓥鐣ャ€佹洿鏂伴€氶亾閿佸畾 |
| 鎺㈢储 | AI 杈呭姪銆佷唬鐞嗘姄鍖呫€侀暱灏惧崗璁紙Dubbo/Thrift/IoT锛?|

**閫€鍑?*锛氬洟闃熷彲鍏变韩闈炴晱鎰熼」鐩祫浜э紱绉佹湁鍖栨渶灏忛儴缃插彲璺戦€氬悓姝ャ€?

---

## 9. 鑴氭湰鐢熷懡鍛ㄦ湡锛堣法闃舵涓嶅彉锛?

```text
before_request
鈫?resolve_variables
鈫?build_request
鈫?send_request
鈫?receive_headers
鈫?receive_stream_chunk
鈫?receive_complete
鈫?run_assertions
鈫?extract_variables
鈫?after_response
```

| 闃舵 | 琛屼负 |
|------|------|
| P0 | 鏃犵敤鎴疯剼鏈紱鍐呯疆鍙橀噺鏇挎崲銆佹彁鍙栵紙JSONPath/Header/Cookie锛夈€佹柇瑷€ |
| P1 | QuickJS 鎸傚埌鍚屼竴鐩镐綅锛汫UI/CLI 鍚屽紩鎿?|
| P2 | 缁勭粐绛栫暐闄愬埗鑴氭湰鑳藉姏锛涘璁￠珮椋庨櫓鑴氭湰 |

---

## 10. 寤鸿鐮斿彂鑺傚锛堝弬鑰冩灦鏋?搂17锛?

| 瑙掕壊 | 寤鸿 | 涓昏矗 |
|------|------|------|
| 鎶€鏈礋璐ｄ汉 | 1 | SPI銆佸畨鍏ㄨ竟鐣屻€佽川閲忛棬绂?|
| Rust | 2鈥? | 鍐呮牳銆丏river銆丄gent銆丆LI銆佸悗缁彃浠惰繍琛屾椂 |
| 鍓嶇 | 2 | 宸ヤ綔鍙般€佺紪杈戝櫒銆佹祦寮?UI |
| Java | 0锛圥0锛夆啋 1鈥?锛圥2锛?| 鍗忎綔鏈嶅姟 |
| 娴嬭瘯 | 1 | 鍗忚鐜銆佸绾︺€佷笁骞冲彴鍐掔儫 |
| 浜у搧/璁捐 | 1 | 鍗忚浼樺厛绾с€侀獙鏀躲€佷氦浜?|

鍗曠嚎绋嬫垨灏忓洟闃熸椂锛屼弗鏍兼寜 **0 鈫?M1 鈫?HTTP 鎵撶（ 鈫?SSE/WS 鈫?鍏朵綑鍗忚 鈫?瀵煎叆** 鍓婂嘲锛岀姝㈠苟琛岄摵澶 Driver銆?

---

## 11. 椋庨櫓涓庡緟鎷嶆澘锛堜粛寮€鏀撅級

| 浜嬮」 | 寤鸿 |
|------|------|
| GraphQL 璁㈤槄鏄惁纭崱 MVP | 鑻ユ帓鏈熺揣锛孧VP 鍙彧鍋?HTTP GraphQL锛岃闃呴『寤?P1 |
| 椤圭洰鏂囦欢锛歋QLite vs `*.protovoy.json` | **娣峰悎**锛氳繍琛屾湡 SQLite锛屽垎浜?Git 鐢?JSON 鍖?|
| OAuth 2.0 鏄惁杩?P0 | 寤鸿 P0 鍋?Bearer/API Key锛汷Auth 鎺堟潈鐮佹祦鏀?P0 鏈垨 P1 |
| 鐢熶骇鐜缁勭粐绾у鎵?| 涓汉妯″紡浜屾纭鍗冲彲锛涚粍缁囩瓥鐣ラ殢 P2 |
| QuickJS vs 鍏朵粬杩愯鏃?| 缁存寔 ADR锛歅1 PoC 鍚庡啀鍐荤粨 |
| 涓夊钩鍙板彂甯冧汉鍔?| MVP 鍐呮祴鍙厛 Win+macOS锛汱inux 绱ч殢锛孉C-01 鍙戝竷鍓嶈ˉ榻?|

---

## 12. 涓嬩竴杩唬锛堝缓璁?1鈥? 鍛級

> **M1 宸叉敹鍙?*锛?026-08-05锛夛細鍐呮牳鎸佷箙鍖?/ 鍙橀噺 / Secret / 璁よ瘉 / 鏂█ / 鍘嗗彶 / 閿欒妯″瀷鍧囪揪閫€鍑烘潯浠躲€?
> 寤鸿涓嬩竴杩唬杩涘叆 **M2**锛氬寮?HTTP锛圕ookie/浠ｇ悊/閲嶅畾鍚戯級骞跺惎鍔?SSE / WebSocket Driver銆?

1. `driver-http`锛欳ookie銆佷唬鐞嗐€侀噸瀹氬悜绛栫暐銆佹祦寮?UI 濂戠害娴嬭瘯
2. 鏂板缓 `driver-sse` 鏈€灏忛€氳矾
3. 鏂板缓 `driver-websocket` 鏈€灏忛€氳矾
4. Protocol Editor Host 楠ㄦ灦锛堟寜 Driver Descriptor 鍒囨崲锛?

---

## 13. 鏂囨。绱㈠紩

| 鏂囨。 | 璺緞 |
|------|------|
| 浜у搧闇€姹?PRD V1.1 | 鍐呴儴鏂囨。锛堟湭鍏紑锛夛紱鍏紑瑙勫垝瑙?[`docs/ARCHITECTURE.md`](./ARCHITECTURE.md) 涓庢湰璁″垝 |
| 鎶€鏈灦鏋?V1.0 | 鍐呴儴鏂囨。锛堟湭鍏紑锛夛紱鍏紑鏋舵瀯瑙?[`docs/ARCHITECTURE.md`](./ARCHITECTURE.md) 涓?[`docs/adr/`](./adr/) |
| 鍝佺墝 | [`docs/BRANDING.md`](./BRANDING.md) |
| ADR | [`docs/adr/`](./adr/) |
| 鏈鍒?| [`docs/PHASE_PLAN.md`](./PHASE_PLAN.md) |
