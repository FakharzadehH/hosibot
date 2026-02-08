# Hosibot: PHP to Go Migration Plan

**Version:** 1.0  
**Date:** 2026-02-08  
**Status:** Draft

---

## Executive Summary

This document provides a comprehensive migration strategy for **Hosibot**, a Telegram VPN bot system, from PHP to Go using the **Echo web framework** and **tucnak/telebot** library. The system manages VPN services across multiple panel types (Marzban, X-UI, Hiddify, WGDashboard, etc.), handles payments, user management, and provides admin capabilities.

### Key Objectives

- Migrate from PHP to Go while maintaining feature parity
- Improve performance, reliability, and maintainability
- Enable horizontal scaling and better resource management
- Maintain backward compatibility during transition
- Zero downtime migration

---

## Table of Contents

1. [Current System Analysis](#1-current-system-analysis)
2. [Target Architecture](#2-target-architecture)
3. [Technology Stack](#3-technology-stack)
4. [Project Structure](#4-project-structure)
5. [Migration Strategy](#5-migration-strategy)
6. [Module-by-Module Migration Plan](#6-module-by-module-migration-plan)
7. [Database Migration](#7-database-migration)
8. [API Migration](#8-api-migration)
9. [Testing Strategy](#9-testing-strategy)
10. [Deployment Plan](#10-deployment-plan)
11. [Risk Assessment](#11-risk-assessment)
12. [Timeline & Milestones](#12-timeline--milestones)

---

## 1. Current System Analysis

### 1.1 Core Components

#### PHP Application Structure

```
hosibot/
├── index.php              # Main bot webhook handler (7497 lines)
├── config.php             # Database & API configuration
├── botapi.php             # Telegram Bot API wrapper
├── function.php           # Core utility functions (2047 lines)
├── keyboard.php           # Keyboard UI builder (1679 lines)
├── panels.php             # Panel management class (2169 lines)
├── request.php            # HTTP client wrapper
├── Marzban.php            # Marzban panel integration (484 lines)
├── x-ui_single.php        # X-UI panel integration
├── hiddify.php            # Hiddify panel integration
├── marzneshin.php         # Marzneshin panel integration
├── WGDashboard.php        # WireGuard Dashboard integration
├── s_ui.php               # S-UI panel integration
├── mikrotik.php           # MikroTik integration
├── ibsng.php              # IBSNG integration
├── alireza_single.php     # Alireza single integration
└── jdf.php                # Jalali date functions

api/                       # REST API endpoints
├── product.php            # Product management API (482 lines)
├── users.php              # User management API (788 lines)
├── service.php            # Service management API
├── payment.php            # Payment API
├── invoice.php            # Invoice API
├── panels.php             # Panel API
├── discount.php           # Discount API
├── category.php           # Category API
└── settings.php           # Settings API

panel/                     # Admin web panel
├── index.php              # Dashboard
├── user.php               # User management
├── product.php            # Product management
├── service.php            # Service management
├── payment.php            # Payment management
└── login.php              # Authentication

payment/                   # Payment gateways
├── zarinpal.php           # ZarinPal gateway
├── iranpay1.php           # IranPay gateway
├── card.php               # Card-to-card payment
├── nowpayment.php         # NOWPayments crypto
├── tronado.php            # Tronado crypto
└── aqayepardakht.php      # Aqaye Pardakht gateway

cronbot/                   # Scheduled tasks
├── expireagent.php        # Agent expiration checker
├── on_hold.php            # On-hold status handler
├── payment_expire.php     # Payment expiration
├── uptime_panel.php       # Panel uptime monitor
├── uptime_node.php        # Node uptime monitor
├── gift.php               # Gift distribution
├── lottery.php            # Lottery system
└── backupbot.php          # Backup automation

vpnbot/                    # Multi-bot support
└── update/
    ├── index.php          # Sub-bot webhook handler
    └── keyboard.php       # Sub-bot keyboard
```

### 1.2 Key Features

- **Multi-panel VPN Management**: Marzban, X-UI, Hiddify, Marzneshin, WGDashboard, S-UI, MikroTik, IBSNG
- **Payment Gateways**: ZarinPal, IranPay, NOWPayments (crypto), Tronado (TRX), Card-to-Card
- **User Management**: Registration, verification, balance, affiliates, agents
- **Product Management**: Multiple products with categories, pricing, volume/time limits
- **Service Lifecycle**: Creation, renewal, expiration, data limit reset
- **Admin Panel**: Web-based dashboard for management
- **Cron Jobs**: Payment expiration, agent expiration, uptime monitoring, backups
- **Multi-language**: JSON-based text customization
- **Telegram Features**: Inline keyboards, callbacks, channel verification

### 1.3 Database Schema

**Core Tables:**

- `user` - User accounts with balance, agent status, affiliates
- `invoice` - Service purchases and subscriptions
- `product` - VPN service products
- `service_other` - Additional services
- `Payment_report` - Payment transactions
- `marzban_panel` - VPN panel configurations
- `admin` - Admin users with roles
- `setting` - Bot configuration
- `textbot` - Customizable text/language
- `channels` - Required Telegram channels
- `Discount` - Discount codes
- `card_number` - Card-to-card payment cards
- `topicid` - Forum topic IDs for reports

### 1.4 Current Issues & Pain Points

1. **Performance**: Single-threaded PHP, no connection pooling
2. **Scalability**: Difficult to scale horizontally
3. **Code Quality**: Large monolithic files (7000+ lines), tight coupling
4. **Error Handling**: Inconsistent error handling patterns
5. **Testing**: No unit tests, difficult to test
6. **Deployment**: Requires web server, PHP runtime, extensions
7. **Concurrency**: No async operations, sequential processing
8. **Type Safety**: Dynamic typing leads to runtime errors

---

## 2. Target Architecture

### 2.1 High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Nginx / Load Balancer                     │
│                   (SSL Termination, Caching)                 │
└──────────────────────────┬──────────────────────────────────┘
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
    ┌───▼────┐        ┌────▼────┐       ┌────▼────┐
    │ Go App │        │ Go App  │       │ Go App  │  (Horizontal Scaling)
    │Instance│        │Instance │       │Instance │
    │  Echo  │        │  Echo   │       │  Echo   │
    └───┬────┘        └────┬────┘       └────┬────┘
        │                  │                  │
        └──────────────────┼──────────────────┘
                           │
        ┌──────────────────┼──────────────────────────────┐
        │                  │                              │
    ┌───▼─────┐       ┌────▼────┐      ┌────────────────▼────┐
    │  MySQL  │       │  Redis  │      │  Telegram Bot API   │
    │ Primary │       │(Cache + │      │  (Webhook/Polling)  │
    │         │       │ Session)│      └─────────────────────┘
    └─┬───────┘       └─────────┘
      │
  ┌───▼───────┐
  │   MySQL   │
  │  Replica  │
  │(Read-only)│
  └───────────┘
```

### 2.2 Layered Architecture

```
┌─────────────────────────────────────────┐
│         Presentation Layer              │
│  (HTTP Handlers, Bot Commands)          │
├─────────────────────────────────────────┤
│         Application Layer               │
│  (Business Logic, Services)             │
├─────────────────────────────────────────┤
│         Domain Layer                    │
│  (Entities, Value Objects)              │
├─────────────────────────────────────────┤
│         Infrastructure Layer            │
│  (Database, External APIs, Cache)       │
└─────────────────────────────────────────┘
```

---

## 3. Technology Stack

### 3.1 Core Framework

**Echo v4.x** - `github.com/labstack/echo/v4`

- High performance, minimal memory footprint
- Built-in middleware (JWT, CORS, Logger, Recover)
- Request validation and binding
- Route grouping and versioning
- Context-based request handling

### 3.2 Telegram Bot

**tucnak/telebot v3** - `gopkg.in/telebot.v3`

- Clean, idiomatic Go API
- Webhook and long polling support
- Inline keyboards, buttons, callbacks
- File upload/download support
- Middleware support
- Auto-reconnection

### 3.3 Database & ORM

**GORM v2** - `gorm.io/gorm`

- Auto-migration support
- Preloading (eager loading)
- Transactions with savepoints
- Hooks (before/after create/update/delete)
- Connection pooling
- Prepared statements

**MySQL Driver** - `gorm.io/driver/mysql`

### 3.4 Essential Libraries

| Category      | Library                                  | Purpose                          |
| ------------- | ---------------------------------------- | -------------------------------- |
| Configuration | `github.com/spf13/viper`                 | Config management from files/env |
| Logging       | `go.uber.org/zap`                        | Structured, leveled logging      |
| Validation    | `github.com/go-playground/validator/v10` | Struct validation                |
| HTTP Client   | `github.com/go-resty/resty/v2`           | REST API calls to panels         |
| Redis         | `github.com/redis/go-redis/v9`           | Caching and sessions             |
| Cron          | `github.com/robfig/cron/v3`              | Task scheduling                  |
| JWT           | `github.com/golang-jwt/jwt/v5`           | JWT authentication               |
| UUID          | `github.com/google/uuid`                 | UUID generation                  |
| Dotenv        | `github.com/joho/godotenv`               | Load .env files                  |
| QR Code       | `github.com/skip2/go-qrcode`             | QR code generation               |
| Crypto        | `golang.org/x/crypto/bcrypt`             | Password hashing                 |

---

## 4. Project Structure

````
hosibot-go/
├── cmd/
│   └── server/
│       └── main.go                    # Application entry point
│
├── internal/
│   ├── config/
│   │   ├── config.go                  # Configuration loader
│   │   └── database.go                # DB connection setup
│   │
│   ├── models/                        # Domain models
│   │   ├── user.go
│   │   ├── invoice.go
│   │   ├── product.go
│   │   ├── payment.go
│   │   ├── panel.go
## 5. Migration Strategy

### 5.1 Approach: Strangler Fig Pattern

Run PHP and Go in parallel, gradually shifting traffic to Go endpoints and the bot webhook. This preserves stability and allows incremental validation.

**Routing Plan (Nginx example):**
- `/api/*` → Go (after each API migration)
- `/bot/webhook` → Go (after telebot migration)
- `/payment/*` → Go (after payment gateways are validated)
- `/panel/*` → PHP (until admin panel migration is complete)

**Key Invariants to Preserve:**
- Same database schema
- Same Telegram bot token and webhook URL
- Same API request/response formats (especially `actions` payloads)
- Same payment callback URLs until switched

### 5.2 Migration Phases (Adjusted to Current Codebase)

**Phase 1: Foundation (Week 1-2)**
- Echo app skeleton, config loader, DB connection
- Shared logging and error handling
- Base middleware (auth, CORS, rate limits)

**Phase 2: API Compatibility Layer (Week 3-4)**
- Implement `/api/*` endpoints with current payload format
- Preserve `Token` header validation (`hash.txt` + `APIKEY`)
- Preserve `logs_api` logging

**Phase 3: Telegram Bot Core (Week 5-7)**
- Webhook handler for Telegram updates
- Port keyboard builder logic and `text.json` rendering
- Implement `update`/`select`/`insert` flows with GORM

**Phase 4: Payments (Week 8-9)**
- ZarinPal + NowPayments + Tronado + Card-to-Card
- Reproduce `Payment_report` and invoice flows
- Validate receipts, cashback rules, reports to channel

**Phase 5: Panel Integrations (Week 10-12)**
- Marzban, X-UI, Hiddify, Marzneshin, WGDashboard, S-UI, IBSNG, MikroTik
- Define unified panel interface and adapter clients

**Phase 6: Cron Jobs (Week 13)**
- Port `cronbot/*` logic to Go `cron` package
- Add structured logs and monitoring

**Phase 7: Admin Panel (Week 14-16)**
- Option A: Keep existing PHP admin panel using Go APIs
- Option B: Rewrite admin panel in Go templates or SPA

**Phase 8: Hardening (Week 17-18)**
- Load tests, security checks, full regression

---

## 6. Module-by-Module Migration Plan

### 6.1 Bot Webhook (`index.php`)

**Current Behavior**
- Receives Telegram updates, enforces Telegram IP check
- Registers users on first message
- Routes by `Chat_type`, `chat_member` events, and message steps
- Writes to `log.txt` and updates `user.step`

**Go Plan**
- Echo handler: `POST /bot/webhook`
- Use telebot with `NewBot` and `webhook` or `LongPoller`
- Create `BotService` to encapsulate business logic
- Replace `checktelegramip()` with middleware that validates IP list
- Implement update de-duplication (`isDuplicateUpdate`) with Redis or file cache

### 6.2 Telegram API Wrapper (`botapi.php`)

**Current Behavior**
- `telegram()` function using cURL
- Helpers for send/edit/delete messages, documents

**Go Plan**
- Use telebot for standard methods
- For methods not supported by telebot, implement raw HTTP client with `resty`
- Standardize error handling and retries (e.g., backoff for 429)

### 6.3 Utilities (`function.php`)

**Key Behaviors to Port**
- `select`, `update` helpers with UTF8 handling
- `ensureTableUtf8mb4`, `ensureCardNumberTableSupportsUnicode`
- `generateUUID`, `rate_arze`, `nowPayments`, `trnado`
- `DirectPayment` flow and service provisioning

**Go Plan**
- Replace dynamic SQL with repository methods
- Use transactions for multi-step updates
- Keep `logs_api` and `log.txt` equivalent
- Implement a structured `AuditLog` table (optional, later)

### 6.4 Keyboards (`keyboard.php`)

**Current Behavior**
- Reads `setting.keyboardmain` JSON
- Substitutes language keys from `textbot` and `text.json`
- Builds inline or reply keyboard dynamically

**Go Plan**
- Build a `KeyboardBuilder` in Go
- Cache `textbot` + settings in Redis
- Ensure same layout and callback data mapping

### 6.5 Panel Management (`panels.php`, `Marzban.php`, `x-ui_single.php`, etc.)

**Current Behavior**
- `ManagePanel` class orchestrates user creation across panels
- Each panel has its own HTTP API wrapper

**Go Plan**
- Define a `PanelClient` interface:
   - `CreateUser`, `UpdateUser`, `DeleteUser`
   - `ResetUsage`, `GetUser`, `GetInbounds`, `GetLinks`
- Create adapters for each panel type:
   - `MarzbanClient`, `XUIClient`, `HiddifyClient`, `MarzneshinClient`, `WGDashboardClient`, `SUIClient`, `IBSNGClient`, `MikrotikClient`
- Move panel configuration from DB into `panel` model struct

### 6.6 Payments (`payment/*.php`)

**Current Behavior**
- ZarinPal verification and report notifications
- NOWPayments and Tronado JSON calls
- Card-to-card manual confirmation

**Go Plan**
- Implement gateway interface:
   - `InitiatePayment`, `VerifyPayment`, `ParseCallback`
- Keep callback URLs consistent during transition
- Ensure `Payment_report` fields are updated exactly as before

### 6.7 Cron Jobs (`cronbot/*`)

**Current Behavior**
- `expireagent.php`, `payment_expire.php`, uptime checks
- Sends Telegram notifications to reports

**Go Plan**
- Implement scheduled tasks with `cron` package
- Add structured logs and error alerts

---

## 7. Database Migration

### 7.1 Schema Strategy
- Keep MySQL schema unchanged initially
- Map tables to GORM structs
- Maintain UTF8MB4 consistency (mirroring `ensureTableUtf8mb4`)

### 7.2 Key Tables to Model First
- `user`, `invoice`, `product`, `Payment_report`, `marzban_panel`, `setting`, `textbot`, `admin`, `Discount`, `topicid`

### 7.3 Model Generation
Use `gorm/gen` to generate models, then customize.

---

## 8. API Migration

### 8.1 Compatibility Mode (Important)
Existing clients send JSON payloads with `actions` field. Go APIs must accept the same format to avoid breaking the admin panel and external integrations.

**Example:**
`api/users.php` expects:
```json
{
   "actions": "users",
   "limit": 50,
   "page": 1,
   "q": ""
}
````

Go handlers should parse this payload and respond with the same JSON structure:

```json
{ "status": true, "msg": "Successful", "obj": { ... } }
```

### 8.2 Endpoint Mapping

- `api/users.php` → `POST /api/users`
- `api/product.php` → `POST /api/products`
- `api/service.php` → `POST /api/services`
- `api/payment.php` → `POST /api/payments`
- `api/panels.php` → `POST /api/panels`

### 8.3 Endpoint-by-Endpoint API Specs (Current PHP Behavior)

**Global Requirements (all API endpoints):**

- Header `Token`: must match `hash.txt` or `APIKEY` from `config.php`
- Content-Type: `application/json`
- Body: JSON with `actions` field
- Response format: `{ "status": bool, "msg": string, "obj": any }`
- Logs: inserts into `logs_api` with header, data, ip, actions

#### 8.3.1 `api/users.php`

**Action: `users`**

- Method: `GET`
- Body: `{ "actions": "users", "limit"?: number, "page"?: number, "q"?: string, "agent"?: string }`
- Response: list of users + pagination

**Action: `user`**

- Method: `GET`
- Body: `{ "actions": "user", "chat_id": number }`
- Response: user detail, invoice/payment/service stats, available panls/products

**Action: `user_add`**

- Method: `POST`
- Body: `{ "actions": "user_add", "chat_id": number }`
- Behavior: calls Telegram `getChat`, inserts user with defaults

**Action: `block_user`**

- Method: `POST`
- Body: `{ "actions": "block_user", "chat_id": number, "description": string, "type_block": "block"|"unblock" }`
- Behavior: updates `User_Status` + `description_blocking`

**Action: `verify_user`**

- Method: `POST`
- Body: `{ "actions": "verify_user", "chat_id": number, "type_verify": "1"|"0" }`

**Action: `change_status_user`**

- Method: `POST`
- Body: `{ "actions": "change_status_user", "chat_id": number, "type": "active"|"inactive" }`

**Action: `add_balance` / `withdrawal`**

- Method: `POST`
- Body: `{ "actions": "add_balance", "chat_id": number, "amount": number }`
- Body: `{ "actions": "withdrawal", "chat_id": number, "amount": number }`

**Action: `accept_number`**

- Method: `POST`
- Body: `{ "actions": "accept_number", "chat_id": number }`

**Action: `send_message`**

- Method: `POST`
- Body: `{ "actions": "send_message", "chat_id": number, "text": string, "file"?: base64, "content_type"?: string }`
- Notes: supports image/video/pdf/audio based on `content_type`

**Action: `set_limit_test`**

- Method: `POST`
- Body: `{ "actions": "set_limit_test", "chat_id": number, "limit_test": number }`

**Action: `transfer_account`**

- Method: `POST`
- Body: `{ "actions": "transfer_account", "chat_id": number, "new_userid": number }`
- Behavior: transfers user and updates related tables

**Action: `join_channel_exception`**

- Method: `POST`
- Body: `{ "actions": "join_channel_exception", "chat_id": number }`

**Action: `cron_notif` / `manage_show_cart` / `zero_balance`**

- Method: `POST`
- Body: `{ "actions": "cron_notif", "chat_id": number, "type": "1"|"0" }`
- Body: `{ "actions": "manage_show_cart", "chat_id": number, "type": "1"|"0" }`
- Body: `{ "actions": "zero_balance", "chat_id": number }`

**Action: `affiliates_users`**

- Method: `GET`
- Body: `{ "actions": "affiliates_users", "chat_id": number }`

**Action: `remove_affiliates` / `remove_affiliate_user`**

- Method: `POST`
- Body: `{ "actions": "remove_affiliates", "chat_id": number }`
- Body: `{ "actions": "remove_affiliate_user", "chat_id": number }`

**Action: `set_agent` / `set_expire_agent` / `set_becoming_negative` / `set_percentage_discount`**

- Method: `POST`
- Body: `{ "actions": "set_agent", "chat_id": number, "agent_type": string }`
- Body: `{ "actions": "set_expire_agent", "chat_id": number, "expire_time": number }`
- Body: `{ "actions": "set_becoming_negative", "chat_id": number, "amount": number }`
- Body: `{ "actions": "set_percentage_discount", "chat_id": number, "percentage": number }`

**Action: `active_bot_agent` / `remove_agent_bot`**

- Method: `POST`
- Body: `{ "actions": "active_bot_agent", "chat_id": number, "token": string }`
- Body: `{ "actions": "remove_agent_bot", "chat_id": number }`

**Action: `set_price_volume_agent_bot` / `set_price_time_agent_bot`**

- Method: `POST`
- Body: `{ "actions": "set_price_volume_agent_bot", "chat_id": number, "amount": number }`
- Body: `{ "actions": "set_price_time_agent_bot", "chat_id": number, "amount": number }`

**Action: `SetPanelAgentShow` / `SetLimitChangeLocation`**

- Method: `POST`
- Body: `{ "actions": "SetPanelAgentShow", "chat_id": number, "panels": object }`
- Body: `{ "actions": "SetLimitChangeLocation", "chat_id": number, "Limit": number }`

#### 8.3.2 `api/product.php`

**Action: `products`**

- Method: `GET`
- Body: `{ "actions": "products", "limit"?: number, "page"?: number, "q"?: string }`

**Action: `product`**

- Method: `GET`
- Body: `{ "actions": "product", "id": number }`
- Response: product detail + panel/category lists + invoice stats

**Action: `product_add`**

- Method: `POST`
- Body: `{ "actions": "product_add", "name": string, "price": number, "data_limit": number, "time": number, "location": string, "agent"?: string, "note"?: string, "data_limit_reset"?: string, "inbounds"?: any, "proxies"?: any, "category"?: string, "one_buy_status"?: number, "hide_panel"?: object }`

**Action: `product_edit`**

- Method: `POST`
- Body: `{ "actions": "product_edit", "id": number, "name"?: string, "price"?: number, "volume"?: number, "time"?: number, "location"?: string, "agent"?: string, "note"?: string, "data_limit_reset"?: string, "inbounds"?: any, "proxies"?: any, "category"?: string, "one_buy_status"?: number, "hide_panel"?: object }`

**Action: `product_delete`**

- Method: `POST`
- Body: `{ "actions": "product_delete", "id": number }`

**Action: `set_inbounds` / `remove_inbounds`**

- Method: `POST`
- Body: `{ "actions": "set_inbounds", "id": number, "input": string }`
- Body: `{ "actions": "remove_inbounds", "id": number }`

#### 8.3.3 `api/service.php`

**Action: `services`**

- Method: `GET`
- Body: `{ "actions": "services", "limit"?: number }`
- Response: rows from `service_other`

#### 8.3.4 `api/payment.php`

**Action: `payments`**

- Method: `GET`
- Body: `{ "actions": "payments", "limit"?: number, "page"?: number, "q"?: string }`

**Action: `payment`**

- Method: `GET`
- Body: `{ "actions": "payment", "id_order": string }`

#### 8.3.5 `api/invoice.php`

**Action: `invoices`**

- Method: `GET`
- Body: `{ "actions": "invoices", "limit"?: number, "page"?: number, "q"?: string }`

**Action: `services`**

- Method: `GET`
- Body: `{ "actions": "services", "limit"?: number, "page"?: number, "q"?: string }`

**Action: `invoice`**

- Method: `GET`
- Body: `{ "actions": "invoice", "id_invoice": string }`

**Action: `remove_service`**

- Method: `POST`
- Body: `{ "actions": "remove_service", "id_invoice": string, "type": "one"|"tow"|"three", "amount"?: number }`

**Action: `invoice_add`**

- Method: `POST`
- Body: `{ "actions": "invoice_add", "chat_id": number, "username": string, "code_product": string, "location_code": string, "time_service"?: number, "volume_service"?: number }`

**Action: `change_status_config`**

- Method: `POST`
- Body: `{ "actions": "change_status_config", "id_invoice": string }`

**Action: `extend_service_admin`**

- Method: `POST`
- Body: `{ "actions": "extend_service_admin", "id_invoice": string, "time_service": number, "volume_service": number }`

#### 8.3.6 `api/panels.php`

**Action: `panels`**

- Method: `GET`
- Body: `{ "actions": "panels", "limit"?: number, "page"?: number, "q"?: string }`

**Action: `panel`**

- Method: `GET`
- Body: `{ "actions": "panel", "id": number }`

**Action: `panel_add` / `panel_edit` / `panel_delete`**

- Method: `POST`
- Body: `{ "actions": "panel_add", "name": string, "price": number, "data_limit": number, "time": number, "location": string, "agent"?: string, "note"?: string, "data_limit_reset"?: string, "inbounds"?: any, "proxies"?: any, "category"?: string, "one_buy_status"?: number, "hide_panel"?: object }`
- Body: `{ "actions": "panel_edit", "id": number, "name"?: string, "sublink"?: string, "config"?: string, "status"?: string, "location"?: string, "agent"?: string, "note"?: string, "data_limit_reset"?: string, "inbounds"?: any, "proxies"?: any, "category"?: string, "one_buy_status"?: number, "hide_panel"?: object }`
- Body: `{ "actions": "panel_delete", "id": number }`

**Action: `set_inbounds`**

- Method: `POST`
- Body: `{ "actions": "set_inbounds", "id": number, "input": string }`

#### 8.3.7 `api/discount.php`

**Action: `discounts` / `discount`**

- Method: `GET`
- Body: `{ "actions": "discounts", "limit"?: number, "page"?: number, "q"?: string }`
- Body: `{ "actions": "discount", "id": number }`

**Action: `discount_add` / `discount_delete`**

- Method: `POST`
- Body: `{ "actions": "discount_add", "code": string, "price": number, "limit_use": number }`
- Body: `{ "actions": "discount_delete", "id": number }`

**Action: `discount_sell_lists` / `discount_sell`**

- Method: `GET`
- Body: `{ "actions": "discount_sell_lists", "limit"?: number, "page"?: number, "q"?: string }`
- Body: `{ "actions": "discount_sell", "id": number }`

**Action: `discount_sell_add` / `discount_sell_delete`**

- Method: `POST`
- Body: `{ "actions": "discount_sell_add", "code": string, "percent": number, "limit_use": number, "agent"?: string, "usefirst"?: string, "useuser"?: string, "code_product"?: string, "code_panel"?: string, "time"?: string, "type"?: string }`
- Body: `{ "actions": "discount_sell_delete", "id": number }`

#### 8.3.8 `api/category.php`

**Action: `categorys` / `category`**

- Method: `GET`
- Body: `{ "actions": "categorys", "limit"?: number, "page"?: number, "q"?: string }`
- Body: `{ "actions": "category", "id": number }`

**Action: `category_add` / `category_edit` / `category_delete`**

- Method: `POST`
- Body: `{ "actions": "category_add", "remark": string }`
- Body: `{ "actions": "category_edit", "id": number, "remark"?: string }`
- Body: `{ "actions": "category_delete", "id": number }`

#### 8.3.9 `api/settings.php`

**Action: `keyboard_set`**

- Method: `POST`
- Body: `{ "actions": "keyboard_set", "keyboard"?: array, "keyboard_reset"?: boolean }`

**Action: `setting_info`**

- Method: `GET`
- Body: `{ "actions": "setting_info" }`

**Action: `save_setting_shop`**

- Method: `POST`
- Body: `{ "actions": "save_setting_shop", "data": [ { "name_value": string, "value": any, "type": "shop"|"general", "json"?: boolean } ] }`

---

## 9. Testing Strategy

### 9.1 Unit Tests

- Services: user, product, payment, panel
- Mock panel HTTP responses

### 9.2 Integration Tests

- API endpoint tests with test DB
- Payment verification mocks

### 9.3 Bot Flow Tests

- Simulate Telegram updates
- Validate state transitions and keyboard responses

---

## 10. Deployment Plan

### 10.1 Go Service

- Build static binary
- Use systemd or Docker

### 10.2 Webhook

- Set Telegram webhook to `https://domain/bot/webhook`
- Support fallback to long polling

### 10.3 Rollout

- Canary deployment with partial traffic
- Monitor errors and latency

---

## 11. Risk Assessment

| Risk                 | Impact | Mitigation                     |
| -------------------- | ------ | ------------------------------ |
| Payment mismatch     | High   | Parallel verification with PHP |
| Panel API changes    | Medium | Contract tests per panel type  |
| Telegram update loss | Medium | Deduplication + retries        |
| Schema mismatch      | High   | Strict migration + backups     |

---

## 12. Timeline & Milestones

| Phase             | Duration | Deliverable            |
| ----------------- | -------- | ---------------------- |
| Foundation        | 2 weeks  | Echo app + DB + config |
| API Compatibility | 2 weeks  | All `/api/*` endpoints |
| Bot Core          | 3 weeks  | Telegram bot in Go     |
| Payments          | 2 weeks  | Gateway migrations     |
| Panels            | 3 weeks  | Panel adapters         |
| Cron Jobs         | 1 week   | Background tasks       |
| Admin Panel       | 3 weeks  | PHP or Go admin        |
| Hardening         | 2 weeks  | Load + security tests  |

- Version control all migrations

### Configuration Management

**File: `config/config.go`**

```go
type Config struct {
    Server   ServerConfig
    Database DatabaseConfig
    Bot      BotConfig
    Payment  PaymentConfig
    Redis    RedisConfig
    JWT      JWTConfig
}
```

**Environment Variables (`.env`)**

```
DB_HOST=localhost
DB_PORT=3306
DB_USER=hosibot
DB_PASSWORD=secret
DB_NAME=hosibot
BOT_TOKEN=your_telegram_token
JWT_SECRET=your_secret
REDIS_URL=redis://localhost:6379
```

---

## Testing Strategy

### Unit Testing

- Test each service function independently
- Mock database and external APIs
- Aim for >80% code coverage
- Use `testing` package and `testify` for assertions

### Integration Testing

- Test API endpoints with real database
- Use test fixtures and database rollback
- Test payment gateway integrations
- Test bot command handling

### Load Testing

- Compare performance: PHP vs Go
- Benchmark critical paths
- Test horizontal scaling capabilities
- Use `k6` or `JMeter` for load testing

### Example Test Structure

```go
// services/user_service_test.go
func TestGetUser(t *testing.T) {
    // Arrange
    mockDB := setupMockDB()
    service := NewUserService(mockDB)

    // Act
    user, err := service.GetUser(1)

    // Assert
    assert.NoError(t, err)
    assert.Equal(t, "username", user.Name)
}
```

---

## Performance Considerations

### Expected Improvements

- **Startup Time**: ~500ms (Go) vs ~2s (PHP)
- **Request Latency**: 10-50% faster
- **Memory Usage**: ~50-80% reduction
- **Throughput**: 2-5x higher requests/sec
- **Concurrent Connections**: Better handling with goroutines

### Optimization Strategies

1. **Database**
   - Connection pooling (GORM default)
   - Query optimization and indexing
   - Read replicas for scaling

2. **Caching**
   - Redis for session management
   - Cache frequently accessed data
   - Implement cache invalidation strategy

3. **Concurrency**
   - Use goroutines for parallel processing
   - Worker pools for background jobs
   - Avoid blocking operations

4. **Compression**
   - Enable gzip compression in Echo
   - Minimize JSON response sizes

---

## Risk Mitigation

### Identified Risks

| Risk                                 | Probability | Impact   | Mitigation                                          |
| ------------------------------------ | ----------- | -------- | --------------------------------------------------- |
| Data loss during migration           | Low         | Critical | Backup DB before migration, validate data integrity |
| Bot API breaking changes             | Low         | High     | Use stable library versions, test thoroughly        |
| Payment gateway issues               | Medium      | Critical | Parallel testing with PHP system, rollback plan     |
| Performance regression               | Medium      | High     | Load testing, benchmarking before go-live           |
| Resource constraints                 | Medium      | Medium   | Gradual rollout, monitoring                         |
| Third-party API integration failures | Low         | Medium   | Proper error handling, fallback mechanisms          |

### Rollback Strategy

1. Keep PHP system running in parallel for 2-4 weeks
2. Route percentage of traffic to Go (canary deployment)
3. Monitor error rates and performance metrics
4. Quick rollback to PHP if critical issues arise

---

## Timeline & Resources

### Timeline Summary

```
Phase 1:   Week 1-2   (Setup & Foundation)
Phase 2:   Week 3-5   (Core API)
Phase 3:   Week 6-8   (Telegram Bot)
Phase 4:   Week 9-11  (Panel & Admin)
Phase 5:   Week 12    (Cron Jobs)
Phase 6:   Week 13-14 (Payment Processing)
Phase 7:   Week 15-16 (VPN Integrations)
Phase 8:   Week 17-18 (Testing & QA)
Phase 9:   Week 19-20 (Deployment)

Total: 20 weeks (~5 months)
```

### Resource Requirements

- **2-3 Go Developers**: Core development
- **1 DevOps Engineer**: Deployment, monitoring, CI/CD
- **1 QA Engineer**: Testing and validation
- **1 Technical Lead**: Architecture oversight
- **Server Resources**:
  - Development: 2-4 vCPU, 8GB RAM
  - Staging: 4-8 vCPU, 16GB RAM
  - Production: 8+ vCPU, 32GB+ RAM (scalable)

---

## Telegram Bot Library Selection: telebot

### Why telebot?

```
✓ Pure Go implementation
✓ Simple, intuitive API
✓ Webhook and polling support
✓ Good documentation and examples
✓ Active maintenance
✓ Minimal dependencies
```

### Alternative: go-telegram-bot-api

```
✓ More comprehensive API coverage
✓ Extensive documentation
✗ Larger codebase
✗ More dependencies
```

### Recommendation

**Use telebot** for this project due to:

- Simplicity and ease of migration
- Lower memory footprint
- Sufficient feature coverage for current needs
- Better fit for Echo integration

### Example Usage

```go
import "github.com/tucnak/telebot"

b, _ := telebot.NewBot(telebot.Settings{
    Token:  os.Getenv("TELEGRAM_TOKEN"),
    Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
})

b.Handle("/start", func(c telebot.Context) error {
    return c.Send("Welcome!")
})

b.Start()
```

---

## Next Steps

1. **Immediate Actions**:
   - Approve migration plan
   - Set up Go development environment
   - Create GitHub/GitLab repository with structure
   - Set up CI/CD pipeline

2. **Week 1 Tasks**:
   - Initialize Echo project
   - Set up database connectivity
   - Create configuration system
   - Implement logging infrastructure

3. **Team Preparation**:
   - Go language training for team
   - Architecture review with stakeholders
   - Risk assessment meeting
   - Resource allocation

---

## Appendix

### Useful Go Libraries Reference

**Web Framework**

- Echo: https://echo.labstack.com/

**Telegram Bot**

- telebot: https://pkg.go.dev/github.com/tucnak/telebot

**Database**

- GORM: https://gorm.io/

**Utilities**

- Viper (Config): https://github.com/spf13/viper
- Zap (Logging): https://github.com/uber-go/zap
- GoValidator: https://github.com/go-playground/validator
- GoCron: https://github.com/go-co-op/gocron
- Go-Redis: https://github.com/redis/go-redis

**Testing**

- Testify: https://github.com/stretchr/testify

---

## Document History

| Version | Date       | Author         | Changes          |
| ------- | ---------- | -------------- | ---------------- |
| 1.0     | 2026-02-06 | Migration Team | Initial creation |
