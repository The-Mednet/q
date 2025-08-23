# Product Requirements Document: SMTP Relay Load Balancing System

## Executive Summary & Objectives

### Overview
This PRD defines requirements for implementing intelligent load balancing across multiple email providers in Mednet's SMTP relay service. The feature will enable automatic domain selection for generic sender addresses while respecting rate limits and provider constraints.

### Business Objectives
- **Primary**: Enable seamless email distribution across multiple provider workspaces
- **Secondary**: Maximize email throughput while respecting provider rate limits
- **Tertiary**: Improve system resilience through intelligent failover and capacity management

### Success Criteria
- Support for generic domain routing (e.g., `brian@invite.com` → multiple backends)
- 99.5% email delivery success rate with load balancing active
- <100ms overhead for domain selection decisions
- Zero configuration required for basic load balancing scenarios

## User Personas & Use Cases

### Primary Personas

**1. Medical Platform Administrator**
- **Need**: Send transactional emails from branded domains without managing complex routing
- **Pain Point**: Manual management of provider quotas and failover scenarios
- **Value**: Automated load distribution with visibility into provider performance

**2. Email Campaign Manager**
- **Need**: Reliable bulk email delivery for medical conferences and announcements
- **Pain Point**: Provider quota exhaustion causing delivery failures
- **Value**: Intelligent capacity management across multiple providers

**3. Development Team**
- **Need**: Simple SMTP interface that abstracts provider complexity
- **Pain Point**: Complex provider-specific configuration and monitoring
- **Value**: Unified interface with automatic provider selection

### Core Use Cases

**UC1: Generic Domain Load Balancing**
```
GIVEN: User sends email from brian@invite.com
WHEN: System receives SMTP request
THEN: System automatically selects optimal workspace from configured providers
AND: Respects rate limits and provider health status
```

**UC2: Capacity-Based Routing**
```
GIVEN: Multiple workspaces configured for load balancing
WHEN: System evaluates routing options
THEN: Selects workspace with highest remaining capacity
AND: Considers both workspace and user-level limits
```

**UC3: Provider Failover**
```
GIVEN: Primary workspace reaches rate limit or fails health check
WHEN: System attempts to route email
THEN: Automatically selects next available workspace
AND: Maintains delivery attempt tracking across providers
```

## Functional Requirements

### Core Load Balancing Features

**FR1: Load Balancing Pool Configuration**
- System MUST support defining load balancing pools containing multiple workspaces
- Pool configuration MUST specify eligible workspaces for automatic selection
- Individual workspaces MUST be able to opt-in/opt-out of load balancing pools
- Pool configuration MUST support weight-based distribution preferences

**FR2: Domain-Based Pool Routing**
- System MUST route emails from configured generic domains through load balancing pools
- Direct domain matches (existing behavior) MUST take precedence over pool routing
- System MUST support multiple pools for different domain patterns

**FR3: Capacity-Aware Selection Algorithm**
- System MUST consider current rate limit usage when selecting workspaces
- Selection algorithm MUST factor in both workspace-level and user-level capacity
- System MUST prefer workspaces with higher remaining capacity percentages
- Algorithm MUST include time-to-reset consideration for rate limit recovery

**FR4: Health-Based Filtering**
- System MUST exclude unhealthy workspaces from load balancing selection
- Health checks MUST include provider API connectivity and authentication status
- Failed workspaces MUST be automatically re-evaluated for health recovery

**FR5: Weighted Distribution**
- System MUST support configurable weights for workspaces within pools
- Higher weighted workspaces MUST receive proportionally more traffic
- Weights MUST be adjustable without service restart

### Configuration & Management

**FR6: Enhanced Workspace Configuration**
- Workspace configuration MUST include load balancing participation settings
- Configuration MUST specify pool membership and weight preferences
- Settings MUST be runtime-configurable through environment variables

**FR7: Pool Management API**
- System MUST provide REST API for pool configuration management
- API MUST support real-time pool membership changes
- API MUST provide pool status and health information

**FR8: Monitoring & Observability**
- System MUST track selection decisions and routing outcomes
- Metrics MUST include per-pool and per-workspace distribution statistics
- Web UI MUST display load balancing status and health information

### Backward Compatibility

**FR9: Existing Behavior Preservation**
- Direct domain-to-workspace mappings MUST continue to function unchanged
- Non-load-balanced workspaces MUST operate exactly as before
- Migration MUST be opt-in with zero impact on existing configurations

## Technical Requirements & Architecture

### System Architecture Changes

**TR1: Load Balancing Engine**
```go
type LoadBalancer interface {
    SelectWorkspace(senderEmail string, pools []LoadBalancingPool) (*config.WorkspaceConfig, error)
    RecordSelection(workspaceID string, success bool)
    GetPoolStatus(poolID string) PoolStatus
}

type LoadBalancingPool struct {
    ID           string                 `json:"id"`
    Name         string                 `json:"name"`
    DomainPatterns []string             `json:"domain_patterns"`
    Workspaces   []PoolWorkspace        `json:"workspaces"`
    Strategy     SelectionStrategy      `json:"strategy"`
    Enabled      bool                   `json:"enabled"`
}

type PoolWorkspace struct {
    WorkspaceID string  `json:"workspace_id"`
    Weight      float64 `json:"weight"`
    Enabled     bool    `json:"enabled"`
}
```

**TR2: Selection Algorithms**
- **Capacity-Weighted**: Combines workspace weight with remaining capacity percentage
- **Round-Robin**: Cycles through healthy workspaces with weight consideration
- **Least-Used**: Selects workspace with lowest current usage percentage
- **Random-Weighted**: Random selection weighted by capacity and configuration

**TR3: Integration Points**
- Modify `workspace.Manager.GetWorkspaceForSender()` to support pool routing
- Enhance `WorkspaceAwareRateLimiter` to provide capacity metrics for selection
- Update SMTP server to use load balancing for eligible domains

### Database Schema Changes

**TR4: Load Balancing Tables**
```sql
CREATE TABLE load_balancing_pools (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    domain_patterns JSON NOT NULL,
    strategy ENUM('capacity_weighted', 'round_robin', 'least_used', 'random_weighted') DEFAULT 'capacity_weighted',
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE TABLE pool_workspaces (
    pool_id VARCHAR(255),
    workspace_id VARCHAR(255),
    weight DECIMAL(5,2) DEFAULT 1.0,
    enabled BOOLEAN DEFAULT TRUE,
    PRIMARY KEY (pool_id, workspace_id),
    FOREIGN KEY (pool_id) REFERENCES load_balancing_pools(id) ON DELETE CASCADE
);

CREATE TABLE load_balancing_selections (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    pool_id VARCHAR(255),
    workspace_id VARCHAR(255),
    sender_email VARCHAR(255),
    selected_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    success BOOLEAN,
    capacity_score DECIMAL(5,4),
    INDEX idx_pool_workspace (pool_id, workspace_id),
    INDEX idx_selected_at (selected_at)
);
```

### Configuration Schema Updates

**TR5: Enhanced Workspace Configuration**
```json
{
  "id": "gmail-workspace",
  "domains": ["joinmednet.org"],
  "display_name": "Gmail Workspace",
  "load_balancing": {
    "enabled": true,
    "pools": ["medical-transactional", "general-notifications"],
    "default_weight": 1.0,
    "min_capacity_threshold": 0.1
  },
  "rate_limits": {
    "workspace_daily": 2000,
    "per_user_daily": 100
  },
  "gmail": {
    "service_account_file": "credentials/service-account.json",
    "enabled": true
  }
}
```

**TR6: Pool Configuration**
```json
{
  "load_balancing_pools": [
    {
      "id": "invite-domain-pool",
      "name": "Invite Domain Distribution",
      "domain_patterns": ["invite.com", "invitations.mednet.org"],
      "strategy": "capacity_weighted",
      "enabled": true,
      "workspaces": [
        {
          "workspace_id": "gmail-workspace-1",
          "weight": 2.0,
          "enabled": true
        },
        {
          "workspace_id": "mailgun-workspace-1", 
          "weight": 1.5,
          "enabled": true
        },
        {
          "workspace_id": "mandrill-workspace-1",
          "weight": 1.0,
          "enabled": true
        }
      ]
    }
  ]
}
```

### Performance Requirements

**TR7: Response Time**
- Domain selection decisions MUST complete within 50ms (95th percentile)
- Pool status updates MUST not block email processing
- Rate limit calculations MUST use cached values when possible

**TR8: Scalability**
- System MUST support up to 50 workspaces per pool
- System MUST support up to 20 pools simultaneously
- Selection algorithm MUST scale linearly with workspace count

## Algorithm Design for Domain Selection

### Capacity-Weighted Selection Algorithm

**Algorithm Overview**
```go
func (lb *LoadBalancer) SelectWorkspace(senderEmail string, pool LoadBalancingPool) (*config.WorkspaceConfig, error) {
    // 1. Filter healthy workspaces
    healthyWorkspaces := lb.filterHealthyWorkspaces(pool.Workspaces)
    
    // 2. Calculate capacity scores
    candidates := []WorkspaceCandidate{}
    for _, ws := range healthyWorkspaces {
        capacity := lb.getWorkspaceCapacity(ws.WorkspaceID, senderEmail)
        if capacity.RemainingPercentage < ws.MinCapacityThreshold {
            continue // Skip if below threshold
        }
        
        score := ws.Weight * capacity.RemainingPercentage
        candidates = append(candidates, WorkspaceCandidate{
            Workspace: ws,
            Score:     score,
            Capacity:  capacity,
        })
    }
    
    // 3. Select using weighted random
    return lb.weightedRandomSelect(candidates)
}
```

**Capacity Calculation**
```go
type CapacityInfo struct {
    WorkspaceRemaining  int
    UserRemaining      int
    WorkspaceLimit     int
    UserLimit          int
    RemainingPercentage float64
    TimeToReset        time.Duration
}

func (lb *LoadBalancer) getWorkspaceCapacity(workspaceID, senderEmail string) CapacityInfo {
    wsSent, wsRemaining, wsReset := lb.rateLimiter.GetWorkspaceStatus(workspaceID)
    userSent, userRemaining, userReset := lb.rateLimiter.GetStatus(workspaceID, senderEmail)
    
    // Use the more restrictive limit
    effectiveRemaining := min(wsRemaining, userRemaining)
    effectiveLimit := min(wsRemaining + wsSent, userRemaining + userSent)
    
    return CapacityInfo{
        WorkspaceRemaining:  wsRemaining,
        UserRemaining:      userRemaining,
        RemainingPercentage: float64(effectiveRemaining) / float64(effectiveLimit),
        TimeToReset:        max(wsReset.Sub(time.Now()), userReset.Sub(time.Now())),
    }
}
```

### Fallback Strategies

**Strategy 1: Graceful Degradation**
- If no pool workspaces available, attempt direct domain mapping
- If direct mapping fails, use global fallback workspace
- Log selection failures for monitoring

**Strategy 2: Capacity Recovery**
- Monitor rate limit reset times for capacity planning
- Temporarily exclude workspaces approaching limits
- Re-evaluate excluded workspaces on rate limit reset

## API Changes & Extensions

### REST API Endpoints

**Pool Management**
```
GET    /api/v1/pools                    # List all pools
POST   /api/v1/pools                    # Create pool
GET    /api/v1/pools/{id}               # Get pool details
PUT    /api/v1/pools/{id}               # Update pool
DELETE /api/v1/pools/{id}               # Delete pool
POST   /api/v1/pools/{id}/test          # Test pool configuration
```

**Pool Membership**
```
POST   /api/v1/pools/{id}/workspaces    # Add workspace to pool
DELETE /api/v1/pools/{id}/workspaces/{workspace_id}  # Remove workspace
PUT    /api/v1/pools/{id}/workspaces/{workspace_id}  # Update workspace settings
```

**Monitoring**
```
GET    /api/v1/pools/{id}/status        # Pool health and capacity
GET    /api/v1/pools/{id}/metrics       # Selection statistics
GET    /api/v1/workspaces/{id}/pools    # Pools containing workspace
```

### Webhook Extensions

**Load Balancing Events**
```json
{
  "event": "load_balancing.workspace_selected",
  "timestamp": "2025-08-22T10:30:00Z",
  "data": {
    "pool_id": "invite-domain-pool",
    "workspace_id": "gmail-workspace-1",
    "sender_email": "brian@invite.com",
    "selection_score": 0.85,
    "capacity_percentage": 0.67,
    "selection_reason": "capacity_weighted"
  }
}

{
  "event": "load_balancing.pool_exhausted",
  "timestamp": "2025-08-22T10:30:00Z", 
  "data": {
    "pool_id": "invite-domain-pool",
    "sender_email": "brian@invite.com",
    "available_workspaces": 0,
    "fallback_action": "direct_mapping"
  }
}
```

## Success Metrics & KPIs

### Primary Metrics

**Delivery Performance**
- **Email Delivery Rate**: >99.5% successful delivery across all pools
- **Selection Latency**: <50ms for 95% of selection decisions
- **Capacity Utilization**: 80-90% of provider limits utilized efficiently

**Load Distribution**
- **Distribution Variance**: <15% deviation from expected weight-based distribution
- **Capacity Efficiency**: Average workspace utilization within 10% of optimal
- **Failover Success Rate**: >99% automatic failover when primary workspace unavailable

### Secondary Metrics

**System Health**
- **Pool Availability**: >99.9% pool availability (at least one healthy workspace)
- **Configuration Drift**: <1% of selections using fallback due to config issues
- **Rate Limit Accuracy**: <2% variance between predicted and actual capacity

**Operational Metrics**
- **Configuration Changes**: Time to apply pool configuration changes <30 seconds
- **Monitoring Coverage**: 100% of selection decisions logged and traceable
- **Alert Accuracy**: <5% false positive rate for capacity and health alerts

### Business Impact Metrics

**Cost Optimization**
- **Provider Cost Distribution**: Even utilization across cost-effective providers
- **Quota Waste Reduction**: <5% of purchased capacity unused due to poor distribution
- **Operational Overhead**: <10% increase in monitoring complexity

**Reliability Improvement**
- **Single Provider Risk**: <20% of total traffic dependent on any single provider
- **Recovery Time**: <5 minutes to restore service after provider failure
- **Capacity Planning**: 95% accuracy in predicting capacity needs

## Implementation Timeline & Milestones

### Phase 1: Foundation (Weeks 1-3)
**Milestone: Core Load Balancing Infrastructure**

*Week 1*
- Design and implement core `LoadBalancer` interface
- Create basic pool configuration data structures
- Implement capacity calculation logic integration with existing rate limiter

*Week 2*
- Develop workspace health checking system
- Implement capacity-weighted selection algorithm
- Create basic pool configuration validation

*Week 3*
- Integrate load balancer with workspace manager
- Implement fallback mechanisms for configuration errors
- Unit tests for core selection logic

**Deliverables:**
- Working load balancer engine with capacity-weighted selection
- Integration with existing rate limiting system
- Comprehensive unit test coverage

### Phase 2: Configuration & Management (Weeks 4-6)
**Milestone: Pool Configuration and Management System**

*Week 4*
- Implement database schema for pools and pool memberships
- Create pool configuration loading from JSON and environment variables
- Develop pool validation and health checking

*Week 5*
- Build REST API endpoints for pool management
- Create pool status and metrics calculation
- Implement real-time pool configuration updates

*Week 6*
- Develop Web UI components for pool management
- Create pool testing and validation tools
- Integration testing with existing workspace configuration

**Deliverables:**
- Complete pool management API
- Web UI for pool configuration and monitoring
- Database-backed pool persistence

### Phase 3: Advanced Features & Optimization (Weeks 7-9)
**Milestone: Production-Ready Load Balancing**

*Week 7*
- Implement multiple selection strategies (round-robin, least-used, random-weighted)
- Add advanced capacity prediction and planning features
- Develop comprehensive monitoring and alerting

*Week 8*
- Performance optimization and caching for selection decisions
- Implement webhook events for load balancing activities
- Create detailed metrics and analytics

*Week 9*
- Load testing and performance validation
- Documentation and operational runbooks
- Production deployment preparation

**Deliverables:**
- Multiple selection algorithms
- Production monitoring and alerting
- Performance optimization and scalability validation

### Phase 4: Production Deployment & Validation (Weeks 10-12)
**Milestone: Production Deployment and Success Validation**

*Week 10*
- Gradual rollout to production with feature flags
- Real-world traffic validation and monitoring
- Performance tuning based on production load

*Week 11*
- Full production deployment
- Success metrics collection and analysis
- User training and documentation

*Week 12*
- Success criteria validation
- Performance review and optimization
- Future enhancement planning

**Deliverables:**
- Successful production deployment
- Validated success metrics achievement
- Operational documentation and team training

## Risk Assessment & Mitigation

### Technical Risks

**Risk: Selection Algorithm Performance Degradation**
- **Probability**: Medium
- **Impact**: High (could block email processing)
- **Mitigation**: 
  - Implement selection decision caching with 60-second TTL
  - Add circuit breaker for algorithm failures with fallback to direct mapping
  - Performance monitoring with automated alerts for >50ms selection times

**Risk: Rate Limiter Integration Complexity**
- **Probability**: Medium  
- **Impact**: Medium (inaccurate capacity calculation)
- **Mitigation**:
  - Extensive integration testing with existing rate limiter
  - Gradual rollout with real-time monitoring of capacity accuracy
  - Fallback to workspace-level limits if user-level calculation fails

**Risk: Configuration Drift and Inconsistency**
- **Probability**: Low
- **Impact**: High (incorrect routing decisions)
- **Mitigation**:
  - Configuration validation on startup and updates
  - Automated configuration backup and rollback mechanisms
  - Real-time configuration consistency monitoring

### Operational Risks

**Risk: Increased Monitoring Complexity**
- **Probability**: High
- **Impact**: Medium (operational overhead)
- **Mitigation**:
  - Automated dashboards for pool health and performance
  - Standardized alerting for common failure scenarios
  - Comprehensive operational documentation and team training

**Risk: Provider-Specific Failure Scenarios**
- **Probability**: Medium
- **Impact**: Medium (partial service degradation)
- **Mitigation**:
  - Provider-specific health checks and automatic exclusion
  - Comprehensive failover testing across all provider combinations
  - Real-time provider status monitoring and alerting

### Business Risks

**Risk: Unexpected Cost Distribution**
- **Probability**: Low
- **Impact**: Medium (budget variance)
- **Mitigation**:
  - Cost monitoring and alerting per provider
  - Weight-based controls for cost-conscious routing
  - Regular cost analysis and optimization reviews

**Risk: Compliance and Audit Requirements**
- **Probability**: Low
- **Impact**: High (regulatory compliance)
- **Mitigation**:
  - Comprehensive selection decision logging
  - Audit trail for all configuration changes
  - Regular compliance review and validation

## Testing & Quality Assurance Strategy

### Unit Testing Strategy

**Core Algorithm Testing**
- Test all selection algorithms with various capacity scenarios
- Validate weight distribution accuracy across different workspace configurations
- Test edge cases: empty pools, single workspace pools, all workspaces at capacity

**Integration Testing**
- Test integration with existing rate limiter for accurate capacity calculation
- Validate workspace manager integration for proper fallback behavior
- Test configuration loading and validation across different sources

### Performance Testing

**Load Testing Scenarios**
- Sustained high-volume email processing with load balancing active
- Peak traffic scenarios with multiple pools and complex routing
- Failover scenarios under high load conditions

**Benchmark Targets**
- Selection decisions: <50ms (95th percentile)
- Configuration updates: <30 seconds to take effect
- Memory usage: <50MB additional overhead per 1000 workspaces

### Integration Testing

**End-to-End Email Flow**
- Complete email flow from SMTP receipt through provider delivery
- Multi-provider failover scenarios with real provider APIs
- Rate limit enforcement across different selection strategies

**Configuration Management**
- Pool configuration updates without service restart
- Workspace addition/removal from active pools
- Migration from direct mapping to pool-based routing

### Monitoring and Alerting Testing

**Health Check Validation**
- Provider connectivity and authentication validation
- Workspace health status accuracy and response time
- Automatic recovery when providers return to healthy state

**Metrics and Analytics**
- Selection decision tracking and analytics accuracy
- Rate limit utilization reporting across pools and workspaces
- Cost and performance optimization recommendations

This PRD provides a comprehensive framework for implementing intelligent load balancing in the SMTP relay service while maintaining the high reliability standards required for medical platform infrastructure. The phased approach ensures controlled rollout with validation at each stage, and the detailed technical specifications provide clear guidance for implementation teams.