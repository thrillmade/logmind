"""Built-in decision templates for common patterns."""

from typing import Dict, List, Optional

TEMPLATES: Dict[str, Dict] = {
    "database": {
        "description": "Database technology selection",
        "reasoning": "Evaluated data model fit, scalability requirements, and operational complexity",
        "alternatives": ["PostgreSQL", "MySQL", "MongoDB", "SQLite", "Redis"],
        "implications": [
            "Connection pooling configuration required",
            "Schema migration strategy needed",
            "Backup and recovery plan required",
        ],
    },
    "api": {
        "description": "API design or technology choice",
        "reasoning": "Considered client needs, performance requirements, and developer experience",
        "alternatives": ["REST", "GraphQL", "gRPC", "WebSockets"],
        "implications": [
            "Versioning strategy needed",
            "Authentication mechanism required",
            "Rate limiting and documentation needed",
        ],
    },
    "architecture": {
        "description": "Architectural pattern or structure decision",
        "reasoning": "Evaluated maintainability, scalability, and team familiarity with the pattern",
        "alternatives": ["Monolith", "Microservices", "Event-driven", "Serverless"],
        "implications": [
            "Affects deployment complexity",
            "Impacts team structure and code ownership",
            "Determines scalability approach",
        ],
    },
    "security": {
        "description": "Security approach or mechanism choice",
        "reasoning": "Assessed threat model, compliance requirements, and implementation complexity",
        "alternatives": ["JWT", "Sessions", "OAuth2", "API keys", "mTLS"],
        "implications": [
            "Regular security audits required",
            "Token refresh and revocation strategy needed",
            "Audit logging required",
        ],
    },
    "performance": {
        "description": "Performance optimization decision",
        "reasoning": "Profiled the bottleneck and evaluated trade-offs against added complexity",
        "alternatives": ["Caching layer", "Database indexing", "CDN", "Query optimization"],
        "implications": [
            "Cache invalidation strategy required",
            "Monitoring and alerting needed",
            "May increase infrastructure costs",
        ],
    },
    "library": {
        "description": "Third-party library or framework selection",
        "reasoning": "Evaluated API quality, maintenance status, community support, and license",
        "alternatives": ["Build in-house", "Alternative libraries"],
        "implications": [
            "Adds an external dependency",
            "Must track upstream security advisories",
            "May require version pinning",
        ],
    },
    "deployment": {
        "description": "Deployment strategy or platform choice",
        "reasoning": "Evaluated operational requirements, team expertise, and cost",
        "alternatives": ["AWS", "GCP", "Azure", "Self-hosted", "Heroku"],
        "implications": [
            "CI/CD pipeline configuration needed",
            "Rollback strategy required",
            "Infrastructure as code recommended",
        ],
    },
}


def get_template(name: str) -> Optional[Dict]:
    """Return a template dict by name, or None if not found."""
    return TEMPLATES.get(name.lower())


def list_templates() -> Dict[str, str]:
    """Return a mapping of template name → description."""
    return {name: t["description"] for name, t in TEMPLATES.items()}
