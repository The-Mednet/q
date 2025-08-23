# SMTP Relay Service - Architecture Diagrams

## Complete System Architecture

### 1. Multi-Layer Architecture Overview

```mermaid
graph TB
    subgraph "Client Layer"
        APP1[Medical Application 1]
        APP2[Medical Application 2]
        APP3[Doctor Portal]
        LEGACY[Legacy Systems]
    end
    
    subgraph "Edge Layer"
        LB[Load Balancer<br/>AWS ALB]
        WAF[Web Application<br/>Firewall]
        DDOS[DDoS Protection<br/>CloudFlare]
    end
    
    subgraph "Service Layer - SMTP Relay"
        subgraph "API Gateway"
            SMTP_GW[SMTP Gateway<br/>:2525]
            REST_API[REST API<br/>:8080]
            WEBHOOK_API[Webhook API<br/>:8081]
        end
        
        subgraph "Core Services"
            PROC1[Processor 1]
            PROC2[Processor 2]
            PROC3[Processor 3]
            ROUTER[Smart Router]
            WORKSPACE_MGR[Workspace<br/>Manager]
        end
        
        subgraph "Provider Abstraction"
            GMAIL_POOL[Gmail<br/>Connection Pool]
            MAILGUN_POOL[Mailgun<br/>Connection Pool]
            FALLBACK[Fallback<br/>Provider]
        end
        
        subgraph "Support Services"
            RATE_LIMITER[Rate Limiter<br/>Service]
            CIRCUIT_BREAKER[Circuit Breaker<br/>Manager]
            HEALTH_MONITOR[Health<br/>Monitor]
            VARIABLE_ENGINE[Variable<br/>Replacement Engine]
            LLM_SERVICE[LLM<br/>Personalization]
        end
    end
    
    subgraph "Data Layer"
        subgraph "Primary Storage"
            MYSQL_PRIMARY[(MySQL<br/>Primary)]
            MYSQL_REPLICA1[(MySQL<br/>Replica 1)]
            MYSQL_REPLICA2[(MySQL<br/>Replica 2)]
        end
        
        subgraph "Cache Layer"
            REDIS_CACHE[(Redis<br/>Cache)]
            MEMCACHED[(Memcached)]
        end
        
        subgraph "Queue Storage"
            QUEUE_DB[(Queue<br/>Tables)]
            RECIPIENT_DB[(Recipient<br/>Tables)]
            AUDIT_DB[(Audit<br/>Tables)]
        end
    end
    
    subgraph "External Services"
        GMAIL_API[Gmail API<br/>Google Workspace]
        MAILGUN_API[Mailgun API]
        OPENAI_API[OpenAI API]
        SENTRY[Sentry<br/>Error Tracking]
        NEWRELIC[New Relic<br/>Monitoring]
    end
    
    subgraph "Monitoring & Observability"
        PROMETHEUS[Prometheus<br/>Metrics]
        GRAFANA[Grafana<br/>Dashboards]
        ELK[ELK Stack<br/>Logging]
        JAEGER[Jaeger<br/>Tracing]
    end
    
    APP1 --> LB
    APP2 --> LB
    APP3 --> LB
    LEGACY --> LB
    
    LB --> WAF
    WAF --> DDOS
    DDOS --> SMTP_GW
    DDOS --> REST_API
    
    SMTP_GW --> QUEUE_DB
    REST_API --> PROC1
    
    PROC1 --> ROUTER
    PROC2 --> ROUTER
    PROC3 --> ROUTER
    
    ROUTER --> WORKSPACE_MGR
    WORKSPACE_MGR --> GMAIL_POOL
    WORKSPACE_MGR --> MAILGUN_POOL
    
    GMAIL_POOL --> GMAIL_API
    MAILGUN_POOL --> MAILGUN_API
    
    PROC1 --> RATE_LIMITER
    RATE_LIMITER --> REDIS_CACHE
    
    ROUTER --> CIRCUIT_BREAKER
    ROUTER --> HEALTH_MONITOR
    
    PROC1 --> LLM_SERVICE
    LLM_SERVICE --> OPENAI_API
    
    MYSQL_PRIMARY --> MYSQL_REPLICA1
    MYSQL_PRIMARY --> MYSQL_REPLICA2
    
    PROC1 --> PROMETHEUS
    PROMETHEUS --> GRAFANA
    
    PROC1 --> SENTRY
    PROC1 --> NEWRELIC
```

### 2. Message Processing Pipeline

```mermaid
graph LR
    subgraph "1. Ingestion"
        RECEIVE[Receive<br/>SMTP Message]
        PARSE[Parse<br/>Headers & Body]
        VALIDATE[Validate<br/>Format]
        EXTRACT[Extract<br/>Metadata]
    end
    
    subgraph "2. Enrichment"
        WORKSPACE[Identify<br/>Workspace]
        RECIPIENT_TRACK[Track<br/>Recipients]
        VARIABLES[Replace<br/>Variables]
        PERSONALIZE[LLM<br/>Personalization]
    end
    
    subgraph "3. Routing"
        STRATEGY[Apply<br/>Strategy]
        SELECT[Select<br/>Provider]
        FAILOVER[Check<br/>Failover]
        BALANCE[Load<br/>Balance]
    end
    
    subgraph "4. Delivery"
        TRANSFORM[Transform<br/>to Provider Format]
        SEND[Send via<br/>Provider API]
        RETRY[Retry<br/>Logic]
        CONFIRM[Confirm<br/>Delivery]
    end
    
    subgraph "5. Post-Processing"
        UPDATE[Update<br/>Status]
        WEBHOOK[Send<br/>Webhooks]
        METRICS[Update<br/>Metrics]
        CLEANUP[Cleanup<br/>Resources]
    end
    
    RECEIVE --> PARSE
    PARSE --> VALIDATE
    VALIDATE --> EXTRACT
    EXTRACT --> WORKSPACE
    WORKSPACE --> RECIPIENT_TRACK
    RECIPIENT_TRACK --> VARIABLES
    VARIABLES --> PERSONALIZE
    PERSONALIZE --> STRATEGY
    STRATEGY --> SELECT
    SELECT --> FAILOVER
    FAILOVER --> BALANCE
    BALANCE --> TRANSFORM
    TRANSFORM --> SEND
    SEND --> RETRY
    RETRY --> CONFIRM
    CONFIRM --> UPDATE
    UPDATE --> WEBHOOK
    WEBHOOK --> METRICS
    METRICS --> CLEANUP
```

### 3. Provider Selection Algorithm

```mermaid
flowchart TD
    START([Message Ready<br/>for Routing])
    
    EXTRACT_SENDER[Extract Sender Email]
    EXTRACT_SENDER --> PARSE_DOMAIN[Parse Domain]
    
    PARSE_DOMAIN --> LOOKUP_WORKSPACE{Workspace<br/>Exists?}
    
    LOOKUP_WORKSPACE -->|No| USE_DEFAULT[Use Default<br/>Workspace]
    LOOKUP_WORKSPACE -->|Yes| LOAD_WORKSPACE[Load Workspace<br/>Configuration]
    
    USE_DEFAULT --> LOAD_WORKSPACE
    
    LOAD_WORKSPACE --> GET_PROVIDERS[Get Available<br/>Providers]
    
    GET_PROVIDERS --> CHECK_ENABLED{Any Providers<br/>Enabled?}
    
    CHECK_ENABLED -->|No| ERROR_NO_PROVIDER[Error: No Providers<br/>Available]
    CHECK_ENABLED -->|Yes| FILTER_HEALTHY[Filter by<br/>Health Status]
    
    FILTER_HEALTHY --> CHECK_CIRCUIT{Circuit Breaker<br/>Status}
    
    CHECK_CIRCUIT --> FILTER_RATE[Filter by<br/>Rate Limits]
    
    FILTER_RATE --> COUNT_AVAILABLE{Available<br/>Count}
    
    COUNT_AVAILABLE -->|0| CHECK_FORCE{Force Send?}
    COUNT_AVAILABLE -->|1| USE_SINGLE[Use Single<br/>Provider]
    COUNT_AVAILABLE -->|>1| APPLY_STRATEGY{Routing<br/>Strategy}
    
    CHECK_FORCE -->|Yes| USE_PRIMARY[Use Primary<br/>Despite Issues]
    CHECK_FORCE -->|No| DEFER_MESSAGE[Defer Message]
    
    APPLY_STRATEGY -->|Priority| SORT_PRIORITY[Sort by Priority]
    APPLY_STRATEGY -->|Round Robin| GET_NEXT_RR[Get Next in<br/>Round Robin]
    APPLY_STRATEGY -->|Weighted| CALC_WEIGHTED[Calculate<br/>Weighted Random]
    APPLY_STRATEGY -->|Least Loaded| FIND_LEAST[Find Least<br/>Loaded]
    APPLY_STRATEGY -->|Domain Based| MATCH_DOMAIN[Match Domain<br/>Pattern]
    
    SORT_PRIORITY --> SELECT_FIRST[Select First]
    GET_NEXT_RR --> INCREMENT_INDEX[Increment Index]
    CALC_WEIGHTED --> RANDOM_SELECT[Random Selection]
    FIND_LEAST --> CHECK_METRICS[Check Metrics]
    MATCH_DOMAIN --> PATTERN_MATCH[Pattern Match]
    
    SELECT_FIRST --> FINAL_PROVIDER[Final Provider<br/>Selected]
    INCREMENT_INDEX --> FINAL_PROVIDER
    RANDOM_SELECT --> FINAL_PROVIDER
    CHECK_METRICS --> FINAL_PROVIDER
    PATTERN_MATCH --> FINAL_PROVIDER
    USE_SINGLE --> FINAL_PROVIDER
    USE_PRIMARY --> FINAL_PROVIDER
    
    FINAL_PROVIDER --> SEND_MESSAGE[Send Message]
    
    SEND_MESSAGE --> SUCCESS{Send<br/>Successful?}
    
    SUCCESS -->|Yes| COMPLETE[Complete]
    SUCCESS -->|No| CHECK_FAILOVER{Failover<br/>Available?}
    
    CHECK_FAILOVER -->|Yes| SELECT_FAILOVER[Select Failover<br/>Provider]
    CHECK_FAILOVER -->|No| MARK_FAILED[Mark Failed]
    
    SELECT_FAILOVER --> SEND_MESSAGE
    
    ERROR_NO_PROVIDER --> END([End])
    DEFER_MESSAGE --> END
    COMPLETE --> END
    MARK_FAILED --> END
```

### 4. Rate Limiting Architecture

```mermaid
graph TB
    subgraph "Rate Limit Layers"
        subgraph "Global Level"
            GLOBAL_LIMIT[Global Daily Limit<br/>100,000 emails/day]
            GLOBAL_HOURLY[Global Hourly<br/>5,000 emails/hour]
            GLOBAL_BURST[Global Burst<br/>100 emails/second]
        end
        
        subgraph "Workspace Level"
            WS_LIMIT_1[Workspace 1<br/>10,000/day]
            WS_LIMIT_2[Workspace 2<br/>5,000/day]
            WS_LIMIT_3[Workspace 3<br/>2,000/day]
        end
        
        subgraph "User Level"
            USER_DEFAULT[Default User<br/>500/day]
            USER_CUSTOM_1[Power User<br/>2,000/day]
            USER_CUSTOM_2[System User<br/>10,000/day]
        end
        
        subgraph "Provider Level"
            GMAIL_LIMIT[Gmail API<br/>2,000/day]
            MAILGUN_LIMIT[Mailgun API<br/>100,000/day]
        end
    end
    
    subgraph "Enforcement Engine"
        CHECK_GLOBAL{Check Global}
        CHECK_WS{Check Workspace}
        CHECK_USER{Check User}
        CHECK_PROVIDER{Check Provider}
        
        ALLOW[Allow Send]
        DEFER[Defer to Queue]
        REJECT[Reject]
    end
    
    subgraph "Tracking Storage"
        REDIS[(Redis Counters)]
        MYSQL[(MySQL Audit)]
    end
    
    MESSAGE[Incoming Message] --> CHECK_GLOBAL
    
    CHECK_GLOBAL -->|Pass| CHECK_WS
    CHECK_GLOBAL -->|Fail| DEFER
    
    CHECK_WS -->|Pass| CHECK_USER
    CHECK_WS -->|Fail| DEFER
    
    CHECK_USER -->|Pass| CHECK_PROVIDER
    CHECK_USER -->|Fail| REJECT
    
    CHECK_PROVIDER -->|Pass| ALLOW
    CHECK_PROVIDER -->|Fail| DEFER
    
    CHECK_GLOBAL --> REDIS
    CHECK_WS --> REDIS
    CHECK_USER --> REDIS
    CHECK_PROVIDER --> REDIS
    
    ALLOW --> MYSQL
    DEFER --> MYSQL
    REJECT --> MYSQL
```

### 5. Circuit Breaker State Machine

```mermaid
stateDiagram-v2
    [*] --> Closed: Initial State
    
    Closed --> Open: Failure Threshold Reached
    Closed --> Closed: Success
    Closed --> Closed: Failure < Threshold
    
    Open --> HalfOpen: Timeout Expired
    Open --> Open: Requests Rejected
    
    HalfOpen --> Closed: Success Threshold Reached
    HalfOpen --> Open: Any Failure
    HalfOpen --> HalfOpen: Success < Threshold
    
    state Closed {
        [*] --> Monitoring
        Monitoring --> Counting: Request
        Counting --> Monitoring: Response
        
        state Counting {
            Success_Count: 0
            Failure_Count: 0
        }
    }
    
    state Open {
        [*] --> Rejecting
        Rejecting --> Waiting: Start Timer
        Waiting --> Timeout: Timer Expires
        
        state Timer {
            Duration: 60s
        }
    }
    
    state HalfOpen {
        [*] --> Testing
        Testing --> Evaluating: Limited Requests
        Evaluating --> Decision: Check Results
        
        state Limits {
            Max_Requests: 10
            Success_Required: 5
        }
    }
```

### 6. Database Relationships

```mermaid
erDiagram
    WORKSPACES ||--o{ MESSAGES : "sends"
    WORKSPACES ||--o{ RECIPIENTS : "manages"
    WORKSPACES ||--o{ RATE_LIMITS : "has"
    WORKSPACES ||--o{ PROVIDERS : "configures"
    
    MESSAGES ||--o{ MESSAGE_RECIPIENTS : "delivers to"
    MESSAGES ||--o{ WEBHOOK_EVENTS : "triggers"
    MESSAGES ||--o{ ATTACHMENTS : "contains"
    MESSAGES ||--o{ MESSAGE_HEADERS : "has"
    
    RECIPIENTS ||--o{ MESSAGE_RECIPIENTS : "receives"
    RECIPIENTS ||--o{ RECIPIENT_EVENTS : "generates"
    RECIPIENTS ||--o{ RECIPIENT_LISTS : "belongs to"
    
    MESSAGE_RECIPIENTS ||--o{ RECIPIENT_EVENTS : "tracks"
    
    PROVIDERS ||--o{ PROVIDER_METRICS : "reports"
    PROVIDERS ||--o{ CIRCUIT_BREAKERS : "monitors"
    PROVIDERS ||--o{ HEALTH_CHECKS : "performs"
    
    CAMPAIGNS ||--o{ MESSAGES : "contains"
    CAMPAIGNS ||--o{ RECIPIENTS : "targets"
    CAMPAIGNS ||--o{ CAMPAIGN_STATS : "tracks"
    
    USERS ||--o{ MESSAGES : "authors"
    USERS ||--o{ USER_RATE_LIMITS : "subject to"
    USERS ||--o{ AUDIT_LOGS : "generates"
    
    WORKSPACES {
        string id PK
        string domain UK
        string display_name
        json configuration
        timestamp created_at
        timestamp updated_at
    }
    
    MESSAGES {
        string id PK
        string workspace_id FK
        string from_email
        text to_emails
        text subject
        longtext body
        enum status
        timestamp queued_at
        timestamp processed_at
    }
    
    RECIPIENTS {
        bigint id PK
        string email UK
        string workspace_id FK
        enum status
        int bounce_count
        json metadata
    }
    
    PROVIDERS {
        string id PK
        string workspace_id FK
        enum type
        json config
        enum status
        int priority
        int weight
    }
    
    CIRCUIT_BREAKERS {
        string provider_id PK
        enum state
        int failure_count
        int success_count
        timestamp last_failure
        timestamp state_changed
    }
```

### 7. Deployment Architecture

```mermaid
graph TB
    subgraph "Production Environment"
        subgraph "AWS Region: us-east-1"
            subgraph "VPC: 10.0.0.0/16"
                subgraph "Public Subnet: 10.0.1.0/24"
                    ALB[Application<br/>Load Balancer]
                    NAT1[NAT Gateway 1]
                    NAT2[NAT Gateway 2]
                end
                
                subgraph "Private Subnet A: 10.0.10.0/24"
                    subgraph "EKS Node Group 1"
                        POD1A[SMTP Relay Pod 1]
                        POD2A[SMTP Relay Pod 2]
                        POD3A[SMTP Relay Pod 3]
                    end
                end
                
                subgraph "Private Subnet B: 10.0.20.0/24"
                    subgraph "EKS Node Group 2"
                        POD1B[SMTP Relay Pod 4]
                        POD2B[SMTP Relay Pod 5]
                        POD3B[SMTP Relay Pod 6]
                    end
                end
                
                subgraph "Database Subnet A: 10.0.30.0/24"
                    RDS_PRIMARY[(RDS MySQL<br/>Primary)]
                    REDIS_PRIMARY[(ElastiCache<br/>Redis Primary)]
                end
                
                subgraph "Database Subnet B: 10.0.40.0/24"
                    RDS_STANDBY[(RDS MySQL<br/>Standby)]
                    REDIS_REPLICA[(ElastiCache<br/>Redis Replica)]
                end
            end
        end
        
        subgraph "AWS Services"
            S3[S3 Buckets<br/>Backups & Logs]
            SECRETS[Secrets Manager<br/>API Keys]
            CLOUDWATCH[CloudWatch<br/>Monitoring]
            ROUTE53[Route 53<br/>DNS]
        end
    end
    
    subgraph "Disaster Recovery: us-west-2"
        DR_RDS[(DR MySQL)]
        DR_PODS[DR Pods<br/>Standby]
    end
    
    subgraph "External"
        INTERNET[Internet]
        GMAIL_DC[Google<br/>Data Centers]
        MAILGUN_DC[Mailgun<br/>Data Centers]
    end
    
    INTERNET --> ROUTE53
    ROUTE53 --> ALB
    ALB --> POD1A
    ALB --> POD2A
    ALB --> POD3A
    ALB --> POD1B
    ALB --> POD2B
    ALB --> POD3B
    
    POD1A --> RDS_PRIMARY
    POD2A --> RDS_PRIMARY
    POD3A --> RDS_PRIMARY
    
    POD1A --> REDIS_PRIMARY
    
    RDS_PRIMARY -.->|Replication| RDS_STANDBY
    REDIS_PRIMARY -.->|Replication| REDIS_REPLICA
    
    RDS_PRIMARY -.->|Cross-Region Backup| DR_RDS
    
    POD1A --> NAT1
    NAT1 --> GMAIL_DC
    NAT1 --> MAILGUN_DC
    
    POD1A --> SECRETS
    POD1A --> S3
    POD1A --> CLOUDWATCH
```

### 8. Security Architecture

```mermaid
graph TB
    subgraph "Security Layers"
        subgraph "Network Security"
            FIREWALL[AWS WAF]
            NACL[Network ACLs]
            SG[Security Groups]
            PRIVATELINK[AWS PrivateLink]
        end
        
        subgraph "Application Security"
            AUTH[Authentication<br/>Service]
            AUTHZ[Authorization<br/>Service]
            RATE_SEC[Rate Limiting]
            INPUT_VAL[Input Validation]
        end
        
        subgraph "Data Security"
            TLS[TLS 1.3<br/>In Transit]
            ENCRYPT_REST[AES-256<br/>At Rest]
            FIELD_ENCRYPT[Field-Level<br/>Encryption]
            TOKENIZATION[PII<br/>Tokenization]
        end
        
        subgraph "Secret Management"
            KMS[AWS KMS]
            SECRETS_MGR[Secrets Manager]
            VAULT[HashiCorp Vault]
            ROTATION[Key Rotation]
        end
        
        subgraph "Compliance"
            AUDIT_LOG[Audit Logging]
            HIPAA[HIPAA Controls]
            PII_SCAN[PII Scanner]
            DLP[Data Loss<br/>Prevention]
        end
    end
    
    subgraph "Threat Detection"
        IDS[Intrusion Detection]
        ANOMALY[Anomaly Detection]
        SIEM[SIEM Integration]
        THREAT_INTEL[Threat Intelligence]
    end
    
    REQUEST[Incoming Request] --> FIREWALL
    FIREWALL --> NACL
    NACL --> SG
    SG --> TLS
    TLS --> AUTH
    AUTH --> AUTHZ
    AUTHZ --> RATE_SEC
    RATE_SEC --> INPUT_VAL
    INPUT_VAL --> APP[Application]
    
    APP --> FIELD_ENCRYPT
    FIELD_ENCRYPT --> DB[(Database)]
    DB --> ENCRYPT_REST
    
    APP --> SECRETS_MGR
    SECRETS_MGR --> KMS
    
    APP --> AUDIT_LOG
    AUDIT_LOG --> SIEM
    
    IDS --> ANOMALY
    ANOMALY --> THREAT_INTEL
    THREAT_INTEL --> SIEM
```

### 9. Monitoring & Observability Stack

```mermaid
graph LR
    subgraph "Data Sources"
        APP[Application<br/>Metrics]
        LOGS[Application<br/>Logs]
        TRACES[Distributed<br/>Traces]
        EVENTS[System<br/>Events]
        CUSTOM[Custom<br/>Metrics]
    end
    
    subgraph "Collection Layer"
        PROM_EXPORT[Prometheus<br/>Exporters]
        FLUENT[Fluentd]
        OTEL[OpenTelemetry<br/>Collector]
        BEATS[Elastic Beats]
    end
    
    subgraph "Storage Layer"
        PROMETHEUS[(Prometheus<br/>TSDB)]
        ELASTIC[(Elasticsearch)]
        JAEGER_STORE[(Jaeger<br/>Storage)]
        S3_LONG[(S3<br/>Long-term)]
    end
    
    subgraph "Processing Layer"
        ALERT_MGR[Alert Manager]
        LOGSTASH[Logstash]
        STREAM[Stream<br/>Processing]
    end
    
    subgraph "Visualization Layer"
        GRAFANA[Grafana<br/>Dashboards]
        KIBANA[Kibana<br/>Log Analysis]
        JAEGER_UI[Jaeger UI<br/>Trace Analysis]
    end
    
    subgraph "Action Layer"
        PAGERDUTY[PagerDuty]
        SLACK[Slack<br/>Notifications]
        AUTO_SCALE[Auto Scaling]
        RUNBOOK[Runbook<br/>Automation]
    end
    
    APP --> PROM_EXPORT
    LOGS --> FLUENT
    TRACES --> OTEL
    EVENTS --> BEATS
    CUSTOM --> PROM_EXPORT
    
    PROM_EXPORT --> PROMETHEUS
    FLUENT --> ELASTIC
    OTEL --> JAEGER_STORE
    BEATS --> ELASTIC
    
    PROMETHEUS --> ALERT_MGR
    ELASTIC --> LOGSTASH
    
    PROMETHEUS --> GRAFANA
    ELASTIC --> KIBANA
    JAEGER_STORE --> JAEGER_UI
    
    ALERT_MGR --> PAGERDUTY
    ALERT_MGR --> SLACK
    ALERT_MGR --> AUTO_SCALE
    ALERT_MGR --> RUNBOOK
    
    PROMETHEUS -.->|Archive| S3_LONG
    ELASTIC -.->|Archive| S3_LONG
```

### 10. High Availability & Failover

```mermaid
graph TB
    subgraph "Primary Region: us-east-1"
        subgraph "Active Components"
            ACTIVE_LB[Active<br/>Load Balancer]
            ACTIVE_PODS[Active Pods<br/>6 instances]
            ACTIVE_DB[(Active<br/>Database)]
            ACTIVE_CACHE[(Active<br/>Cache)]
        end
        
        subgraph "Health Monitoring"
            HEALTH_CHECK[Health Check<br/>Service]
            WATCHDOG[Watchdog<br/>Timer]
        end
    end
    
    subgraph "Standby Region: us-west-2"
        subgraph "Standby Components"
            STANDBY_LB[Standby<br/>Load Balancer]
            STANDBY_PODS[Standby Pods<br/>3 instances]
            STANDBY_DB[(Standby<br/>Database)]
            STANDBY_CACHE[(Standby<br/>Cache)]
        end
    end
    
    subgraph "Failover Controller"
        DETECT[Failure<br/>Detection]
        DECIDE[Failover<br/>Decision]
        EXECUTE[Execute<br/>Failover]
        VERIFY[Verify<br/>Failover]
    end
    
    subgraph "DNS Management"
        ROUTE53_HEALTH[Route 53<br/>Health Checks]
        DNS_FAILOVER[DNS<br/>Failover Policy]
    end
    
    ACTIVE_LB --> HEALTH_CHECK
    HEALTH_CHECK --> WATCHDOG
    WATCHDOG --> DETECT
    
    DETECT -->|Failure| DECIDE
    DECIDE -->|Confirm| EXECUTE
    EXECUTE --> DNS_FAILOVER
    DNS_FAILOVER --> STANDBY_LB
    
    EXECUTE --> VERIFY
    VERIFY -->|Success| STANDBY_PODS
    
    ACTIVE_DB -.->|Continuous Sync| STANDBY_DB
    ACTIVE_CACHE -.->|Snapshot| STANDBY_CACHE
    
    ROUTE53_HEALTH --> ACTIVE_LB
    ROUTE53_HEALTH --> STANDBY_LB
```

### 11. Data Flow for Email Personalization

```mermaid
sequenceDiagram
    participant Client
    participant SMTP
    participant Queue
    participant Processor
    participant Variables
    participant LLM
    participant Provider
    participant Recipient
    
    Client->>SMTP: Send Email Template
    SMTP->>Queue: Enqueue with Variables
    
    Queue->>Processor: Fetch Message
    Processor->>Variables: Extract Variable Placeholders
    Variables-->>Processor: List of Variables
    
    Processor->>Processor: Load Recipient Data
    
    alt Has {{FIRST_NAME}} or {{LAST_NAME}}
        Processor->>Recipient: Get Recipient Info
        Recipient-->>Processor: Name Data
    end
    
    alt Has <<TRENDING_QUESTION>>
        Processor->>Variables: Get Trending Question
        Variables-->>Processor: Current Trending Topic
    end
    
    alt LLM Personalization Enabled
        Processor->>LLM: Send Content for Personalization
        LLM->>LLM: Apply AI Model
        LLM-->>Processor: Personalized Content
    end
    
    Processor->>Variables: Replace All Variables
    Variables-->>Processor: Final Content
    
    Processor->>Provider: Send Personalized Email
    Provider-->>Processor: Delivery Confirmation
    
    Processor->>Queue: Update Status
    Processor->>Recipient: Track Delivery
```

### 12. Workspace Isolation Model

```mermaid
graph TB
    subgraph "Multi-Tenant Architecture"
        subgraph "Workspace: Hospital A"
            WS_A_CONFIG[Configuration]
            WS_A_PROVIDERS[Gmail + Mailgun]
            WS_A_LIMITS[5000 emails/day]
            WS_A_DATA[(Isolated Data)]
            WS_A_USERS[Users: 50]
        end
        
        subgraph "Workspace: Clinic B"
            WS_B_CONFIG[Configuration]
            WS_B_PROVIDERS[Gmail Only]
            WS_B_LIMITS[1000 emails/day]
            WS_B_DATA[(Isolated Data)]
            WS_B_USERS[Users: 10]
        end
        
        subgraph "Workspace: Research Lab C"
            WS_C_CONFIG[Configuration]
            WS_C_PROVIDERS[Mailgun Only]
            WS_C_LIMITS[10000 emails/day]
            WS_C_DATA[(Isolated Data)]
            WS_C_USERS[Users: 100]
        end
    end
    
    subgraph "Shared Infrastructure"
        SHARED_COMPUTE[Compute Resources]
        SHARED_NETWORK[Network Layer]
        SHARED_MONITORING[Monitoring Stack]
    end
    
    subgraph "Isolation Boundaries"
        DATA_ISOLATION[Data Isolation<br/>Row-Level Security]
        CONFIG_ISOLATION[Config Isolation<br/>Workspace Scoped]
        RATE_ISOLATION[Rate Limit Isolation<br/>Per Workspace]
        PROVIDER_ISOLATION[Provider Isolation<br/>Dedicated Credentials]
    end
    
    WS_A_DATA -.->|Isolated| DATA_ISOLATION
    WS_B_DATA -.->|Isolated| DATA_ISOLATION
    WS_C_DATA -.->|Isolated| DATA_ISOLATION
    
    WS_A_CONFIG --> CONFIG_ISOLATION
    WS_B_CONFIG --> CONFIG_ISOLATION
    WS_C_CONFIG --> CONFIG_ISOLATION
    
    WS_A_LIMITS --> RATE_ISOLATION
    WS_B_LIMITS --> RATE_ISOLATION
    WS_C_LIMITS --> RATE_ISOLATION
    
    WS_A_PROVIDERS --> PROVIDER_ISOLATION
    WS_B_PROVIDERS --> PROVIDER_ISOLATION
    WS_C_PROVIDERS --> PROVIDER_ISOLATION
    
    DATA_ISOLATION --> SHARED_COMPUTE
    CONFIG_ISOLATION --> SHARED_COMPUTE
    RATE_ISOLATION --> SHARED_COMPUTE
    PROVIDER_ISOLATION --> SHARED_NETWORK
```

---

## Diagram Legend

### Symbols Used

| Symbol | Meaning |
|--------|---------|
| `[Component]` | Service or Application Component |
| `[(Database)]` | Database or Persistent Storage |
| `{Decision}` | Decision Point in Flow |
| `-->` | Direct Connection/Flow |
| `-.->` | Asynchronous or Optional Connection |
| `subgraph` | Logical Grouping |
| `(Process)` | Start/End Point |

### Color Coding (When Rendered)

- **Blue**: Core Services
- **Green**: Healthy/Active Components
- **Yellow**: Warning/Degraded State
- **Red**: Failed/Inactive Components
- **Gray**: External Services

---

*Document Version: 1.0.0*  
*Last Updated: 2024-01-15*  
*Architecture Diagrams for SMTP Relay Service*