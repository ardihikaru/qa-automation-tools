#!/usr/bin/env python3
"""
Database Setup Script for Neon PostgreSQL
Creates all tables, views, and seed data for the QA Automation Platform.
"""

import os
import sys
import psycopg2
from psycopg2 import sql
from psycopg2.extras import execute_values
from datetime import datetime

# Connection string from environment or use provided
NEON_CONN = os.environ.get('NEON_DB_CONN')
if not NEON_CONN:
    # Try to read from file
    env_path = '_docs/markdown-results/by-roles/.env'
    if os.path.exists(env_path):
        with open(env_path) as f:
            for line in f:
                if line.startswith('NEON_DB_CONN='):
                    NEON_CONN = line.split('=', 1)[1].strip()
                    break

if not NEON_CONN:
    print("❌ Error: NEON_DB_CONN not found in environment or .env file")
    sys.exit(1)

print("📦 Database Setup for QA Automation Platform")
print("=" * 50)

def connect():
    """Connect to Neon database"""
    try:
        conn = psycopg2.connect(NEON_CONN)
        conn.autocommit = True
        print("✅ Connected to Neon")
        return conn
    except Exception as e:
        print(f"❌ Connection failed: {e}")
        sys.exit(1)

def enable_extensions(cur):
    """Enable pgvector extension"""
    print("\n📦 Enabling extensions...")
    
    extensions = [
        "CREATE EXTENSION IF NOT EXISTS vector;",
    ]
    
    for ext in extensions:
        try:
            cur.execute(ext)
            print(f"  ✅ Extension enabled")
        except Exception as e:
            print(f"  ⚠️  {e}")

def create_schemas(cur):
    """Create database schemas"""
    print("\n📦 Creating schemas...")
    
    schemas = [
        ("ai", "AI caching and learning"),
        ("qa", "Issues and skills"),
        ("logging", "Cost tracking and analytics"),
    ]
    
    for schema_name, description in schemas:
        cur.execute(f"CREATE SCHEMA IF NOT EXISTS {schema_name};")
        print(f"  ✅ Schema: {schema_name} ({description})")

def create_core_tables(cur):
    """Create core tables in public schema"""
    print("\n📦 Creating core tables...")
    
    tables = [
        """
        CREATE TABLE IF NOT EXISTS roles (
            id SERIAL PRIMARY KEY,
            name TEXT NOT NULL UNIQUE,
            description TEXT,
            created_at TIMESTAMP DEFAULT NOW()
        );
        """,
        """
        CREATE TABLE IF NOT EXISTS flows (
            id SERIAL PRIMARY KEY,
            role_id INT REFERENCES roles(id),
            name TEXT NOT NULL,
            description TEXT,
            created_at TIMESTAMP DEFAULT NOW(),
            updated_at TIMESTAMP DEFAULT NOW()
        );
        """,
        """
        CREATE TABLE IF NOT EXISTS steps (
            id SERIAL PRIMARY KEY,
            flow_id INT REFERENCES flows(id) ON DELETE CASCADE,
            step_order INT NOT NULL,
            action TEXT NOT NULL,
            page_url TEXT,
            details JSONB,
            created_at TIMESTAMP DEFAULT NOW()
        );
        """,
        """
        CREATE TABLE IF NOT EXISTS analytics (
            id SERIAL PRIMARY KEY,
            feature TEXT NOT NULL,
            usage_count INT DEFAULT 0,
            last_used TIMESTAMP DEFAULT NOW()
        );
        """,
    ]
    
    for table in tables:
        table_name = table.split("CREATE TABLE IF NOT EXISTS ")[1].split(" (")[0]
        cur.execute(table)
        print(f"  ✅ Table: {table_name}")

def create_ai_tables(cur):
    """Create AI tables in ai schema"""
    print("\n📦 Creating AI tables...")
    
    tables = [
        """
        CREATE TABLE IF NOT EXISTS ai.ai_cache (
            id SERIAL PRIMARY KEY,
            cache_key TEXT UNIQUE NOT NULL,
            input_hash TEXT NOT NULL,
            output TEXT NOT NULL,
            model TEXT DEFAULT 'abab6.5s-chat',
            cost_estimate DECIMAL(10, 6) DEFAULT 0,
            created_at TIMESTAMP DEFAULT NOW()
        );
        """,
        """
        CREATE TABLE IF NOT EXISTS ai.embedding_cache (
            id SERIAL PRIMARY KEY,
            input_text TEXT UNIQUE NOT NULL,
            input_hash TEXT NOT NULL,
            embedding vector(1536),
            model TEXT DEFAULT 'emb-embedding-01',
            created_at TIMESTAMP DEFAULT NOW()
        );
        """,
        """
        CREATE TABLE IF NOT EXISTS ai.learning_cache (
            id SERIAL PRIMARY KEY,
            intent TEXT NOT NULL,
            normalized_input TEXT NOT NULL,
            flow_id INT,
            embedding vector(1536),
            action TEXT NOT NULL,
            page TEXT NOT NULL,
            success_score FLOAT DEFAULT 1.0,
            usage_count INT DEFAULT 1,
            last_used TIMESTAMP DEFAULT NOW(),
            created_at TIMESTAMP DEFAULT NOW(),
            updated_at TIMESTAMP DEFAULT NOW()
        );
        """,
        """
        CREATE TABLE IF NOT EXISTS ai.action_sequences (
            id SERIAL PRIMARY KEY,
            intent TEXT NOT NULL,
            steps TEXT[] NOT NULL,
            success_score FLOAT DEFAULT 1.0,
            usage_count INT DEFAULT 0,
            created_at TIMESTAMP DEFAULT NOW()
        );
        """,
        """
        CREATE TABLE IF NOT EXISTS ai.navigation_edges (
            id SERIAL PRIMARY KEY,
            from_page TEXT NOT NULL,
            to_page TEXT NOT NULL,
            action TEXT NOT NULL,
            success_rate FLOAT DEFAULT 0.5,
            usage_count INT DEFAULT 0,
            created_at TIMESTAMP DEFAULT NOW()
        );
        """,
    ]
    
    for table in tables:
        table_name = table.split("CREATE TABLE IF NOT EXISTS ")[1].split(" (")[0]
        cur.execute(table)
        print(f"  ✅ Table: {table_name}")

def create_qa_tables(cur):
    """Create QA tables in qa schema"""
    print("\n📦 Creating QA tables...")
    
    tables = [
        """
        CREATE TABLE IF NOT EXISTS qa.skills (
            id SERIAL PRIMARY KEY,
            name TEXT NOT NULL UNIQUE,
            description TEXT,
            pattern JSONB,
            check_type TEXT,
            check_query TEXT,
            severity TEXT DEFAULT 'medium',
            is_builtin BOOLEAN DEFAULT TRUE,
            is_active BOOLEAN DEFAULT TRUE,
            created_at TIMESTAMP DEFAULT NOW()
        );
        """,
        """
        CREATE TABLE IF NOT EXISTS qa.skill_logs (
            id SERIAL PRIMARY KEY,
            scenario_id TEXT,
            skill_id INT REFERENCES qa.skills(id),
            page TEXT,
            action TEXT,
            passed BOOLEAN,
            details JSONB,
            error_message TEXT,
            created_at TIMESTAMP DEFAULT NOW()
        );
        """,
        """
        CREATE TABLE IF NOT EXISTS qa.learned_skills (
            id SERIAL PRIMARY KEY,
            name TEXT NOT NULL,
            description TEXT,
            pattern JSONB,
            check_type TEXT,
            check_query TEXT,
            frequency INT DEFAULT 1,
            created_at TIMESTAMP DEFAULT NOW()
        );
        """,
        """
        CREATE TABLE IF NOT EXISTS qa.raw_issues (
            id SERIAL PRIMARY KEY,
            scenario_id TEXT,
            page TEXT,
            action TEXT,
            error_type TEXT,
            error_text TEXT,
            dom_snapshot TEXT,
            category TEXT,
            severity TEXT DEFAULT 'medium',
            is_fixed BOOLEAN DEFAULT FALSE,
            notes TEXT,
            created_at TIMESTAMP DEFAULT NOW()
        );
        """,
        """
        CREATE TABLE IF NOT EXISTS qa.issue_suggestions (
            id SERIAL PRIMARY KEY,
            issue_id INT REFERENCES qa.raw_issues(id) ON DELETE CASCADE,
            suggestion TEXT NOT NULL,
            confidence FLOAT DEFAULT 0.5,
            target TEXT,
            file_path TEXT,
            diff TEXT,
            function_name TEXT,
            test_coverage BOOLEAN DEFAULT FALSE,
            priority_score FLOAT DEFAULT 0,
            created_at TIMESTAMP DEFAULT NOW()
        );
        """,
        """
        CREATE TABLE IF NOT EXISTS qa.goal_events (
            id SERIAL PRIMARY KEY,
            scenario_id TEXT,
            goal TEXT,
            confidence FLOAT,
            signals JSONB,
            achieved BOOLEAN DEFAULT FALSE,
            created_at TIMESTAMP DEFAULT NOW()
        );
        """,
    ]
    
    for table in tables:
        table_name = table.split("CREATE TABLE IF NOT EXISTS ")[1].split(" (")[0]
        cur.execute(table)
        print(f"  ✅ Table: {table_name}")

def create_logging_tables(cur):
    """Create logging tables"""
    print("\n📦 Creating logging tables...")
    
    tables = [
        """
        CREATE TABLE IF NOT EXISTS logging.request_logs (
            id SERIAL PRIMARY KEY,
            type TEXT NOT NULL,
            endpoint TEXT,
            method TEXT,
            feature TEXT,
            input_tokens INT DEFAULT 0,
            output_tokens INT DEFAULT 0,
            cost_estimate DECIMAL(10, 6) DEFAULT 0,
            cache_hit BOOLEAN DEFAULT FALSE,
            flow_id INT,
            success BOOLEAN DEFAULT TRUE,
            created_at TIMESTAMP DEFAULT NOW()
        );
        """,
    ]
    
    for table in tables:
        table_name = table.split("CREATE TABLE IF NOT EXISTS ")[1].split(" (")[0]
        cur.execute(table)
        print(f"  ✅ Table: {table_name}")

def create_indexes(cur):
    """Create indexes for performance"""
    print("\n📦 Creating indexes...")
    
    indexes = [
        # AI indexes
        "CREATE INDEX IF NOT EXISTS idx_ai_cache_hash ON ai.ai_cache(input_hash);",
        "CREATE INDEX IF NOT EXISTS idx_embedding_cache_hash ON ai.embedding_cache(input_hash);",
        "CREATE INDEX IF NOT EXISTS idx_learning_intent ON ai.learning_cache(intent);",
        # Learning cache with vector - use ivfflat or hash instead of gin
        "CREATE INDEX IF NOT EXISTS idx_learning_embedding ON ai.learning_cache USING ivfflat(embedding vector_cosine_ops);",
        # Skip problematic indexes - can be added manually if needed
        "CREATE INDEX IF NOT EXISTS idx_nav_from ON ai.navigation_edges(from_page);",
        
        # QA indexes
        "CREATE INDEX IF NOT EXISTS idx_skills_name ON qa.skills(name);",
        "CREATE INDEX IF NOT EXISTS idx_skill_logs_scenario ON qa.skill_logs(scenario_id);",
        "CREATE INDEX IF NOT EXISTS idx_skill_logs_skill ON qa.skill_logs(skill_id);",
        "CREATE INDEX IF NOT EXISTS idx_raw_issues_created ON qa.raw_issues(created_at);",
        "CREATE INDEX IF NOT EXISTS idx_raw_issues_category ON qa.raw_issues(category);",
        "CREATE INDEX IF NOT EXISTS idx_raw_issues_fixed ON qa.raw_issues(is_fixed);",
        "CREATE INDEX IF NOT EXISTS idx_suggestions_issue ON qa.issue_suggestions(issue_id);",
        "CREATE INDEX IF NOT EXISTS idx_goal_scenario ON qa.goal_events(scenario_id);",
        
        # Logging indexes
        "CREATE INDEX IF NOT EXISTS idx_request_logs_created ON logging.request_logs(created_at);",
        "CREATE INDEX IF NOT EXISTS idx_request_logs_type ON logging.request_logs(type);",
        "CREATE INDEX IF NOT EXISTS idx_request_logs_feature ON logging.request_logs(feature);",
    ]
    
    for idx in indexes:
        idx_name = idx.split("CREATE INDEX IF NOT EXISTS ")[1].split(" ON ")[0]
        cur.execute(idx)
        print(f"  ✅ Index: {idx_name}")

def create_views(cur):
    """Create dashboard views"""
    print("\n📦 Creating views...")
    
    views = [
        """
        CREATE OR REPLACE VIEW cost_overview AS
        SELECT
            COUNT(*) as total_requests,
            COALESCE(SUM(cost_estimate), 0) as total_cost,
            COALESCE(AVG(cost_estimate), 0) as avg_cost,
            COALESCE(COUNT(*) FILTER (WHERE cache_hit = true) * 1.0 / NULLIF(COUNT(*), 0), 0) as cache_hit_rate
        FROM logging.request_logs;
        """,
        """
        CREATE OR REPLACE VIEW fix_rate AS
        SELECT
            COUNT(*) FILTER (WHERE is_fixed = true) as fixed,
            COUNT(*) FILTER (WHERE is_fixed = false) as open,
            COALESCE(COUNT(*) FILTER (WHERE is_fixed = true) * 1.0 / NULLIF(COUNT(*), 0), 0) as fix_rate
        FROM qa.raw_issues;
        """,
        """
        CREATE OR REPLACE VIEW issues_by_target AS
        SELECT
            COALESCE(category, 'unknown') as category,
            COUNT(*) as total
        FROM qa.raw_issues
        GROUP BY category;
        """,
        """
        CREATE OR REPLACE VIEW skills_summary AS
        SELECT
            s.name,
            COUNT(*) FILTER (WHERE sl.passed = false) as fail_count,
            COUNT(*) as total,
            COALESCE(COUNT(*) FILTER (WHERE sl.passed = false) * 1.0 / NULLIF(COUNT(*), 0), 0) as fail_rate
        FROM qa.skill_logs sl
        JOIN qa.skills s ON sl.skill_id = s.id
        GROUP BY s.name;
        """,
        """
        CREATE OR REPLACE VIEW goals_summary AS
        SELECT
            goal,
            COUNT(*) as total_attempts,
            COUNT(*) FILTER (WHERE achieved = true) as achieved,
            COALESCE(AVG(confidence), 0) as avg_confidence
        FROM qa.goal_events
        GROUP BY goal;
        """,
        """
        CREATE OR REPLACE VIEW learning_stats AS
        SELECT
            intent,
            action,
            success_score,
            usage_count,
            last_used
        FROM ai.learning_cache
        ORDER BY usage_count DESC
        LIMIT 20;
        """,
    ]
    
    for view in views:
        view_name = view.split("CREATE OR REPLACE VIEW ")[1].split(" AS")[0]
        cur.execute(view)
        print(f"  ✅ View: {view_name}")

def insert_seed_data(cur):
    """Insert initial seed data"""
    print("\n📦 Inserting seed data...")
    
    # Roles
    roles = [
        ("super_admin", "Super Administrator with full system access"),
        ("admin", "Administrator with management access"),
        ("doctor", "Medical professional"),
        ("cashier", "Billing and payment operations"),
    ]
    
    for name, desc in roles:
        cur.execute(
            "INSERT INTO roles (name, description) VALUES (%s, %s) ON CONFLICT (name) DO NOTHING",
            (name, desc)
        )
    print(f"  ✅ Roles: {len(roles)} entries")
    
    # QA Skills (built-in)
    skills = [
        ("max_length", "Check textarea maxlength attribute", "validation", "textarea:not([maxlength])", "high"),
        ("required_field", "Check required field bypass", "validation", "input[required]", "high"),
        ("data_persistence", "Check form data persistence", "data", "form[data-persist]", "medium"),
        ("multi_tab_save", "Check tab data loss", "validation", ".tab-content", "medium"),
        ("input_type", "Check proper input types", "selector", "input:not([type])", "low"),
        ("label_association", "Check label associations", "selector", "input:not([aria-label])", "medium"),
    ]
    
    for name, desc, check_type, check_query, severity in skills:
        cur.execute("""
            INSERT INTO qa.skills (name, description, check_type, check_query, severity, is_builtin)
            VALUES (%s, %s, %s, %s, %s, TRUE)
            ON CONFLICT (name) DO NOTHING
        """, (name, desc, check_type, check_query, severity))
    print(f"  ✅ QA Skills: {len(skills)} entries")
    
    # Analytics features
    features = [
        ("flow_generation", 0),
        ("flow_summarization", 0),
        ("embedding_generation", 0),
        ("semantic_search", 0),
        ("issue_classification", 0),
        ("suggestion_generation", 0),
        ("skill_execution", 0),
        ("goal_detection", 0),
    ]
    
    for feature, count in features:
        cur.execute("""
            INSERT INTO analytics (feature, usage_count) VALUES (%s, %s)
        """, (feature, count))
    print(f"  ✅ Analytics: {len(features)} entries")

def verify_setup(cur):
    """Verify database setup"""
    print("\n📦 Verifying setup...")
    
    # Count tables
    cur.execute("""
        SELECT count(*) FROM information_schema.tables 
        WHERE table_schema IN ('public', 'ai', 'qa', 'logging')
        AND table_type = 'BASE TABLE'
    """)
    table_count = cur.fetchone()[0]
    print(f"  ✅ Tables created: {table_count}")
    
    # Count views
    cur.execute("""
        SELECT count(*) FROM information_schema.views 
        WHERE table_schema IN ('public', 'ai', 'qa', 'logging')
    """)
    view_count = cur.fetchone()[0]
    print(f"  ✅ Views created: {view_count}")
    
    # Count roles
    cur.execute("SELECT count(*) FROM roles")
    role_count = cur.fetchone()[0]
    print(f"  ✅ Roles seeded: {role_count}")
    
    # Count skills
    cur.execute("SELECT count(*) FROM qa.skills")
    skill_count = cur.fetchone()[0]
    print(f"  ✅ Skills seeded: {skill_count}")
    
    return table_count, view_count

def main():
    conn = connect()
    cur = conn.cursor()
    
    try:
        # Execute setup steps
        enable_extensions(cur)
        create_schemas(cur)
        create_core_tables(cur)
        create_ai_tables(cur)
        create_qa_tables(cur)
        create_logging_tables(cur)
        create_indexes(cur)
        create_views(cur)
        insert_seed_data(cur)
        
        # Verify
        table_count, view_count = verify_setup(cur)
        
        print("\n" + "=" * 50)
        print("✅ Database setup complete!")
        print(f"   Tables: {table_count}")
        print(f"   Views: {view_count}")
        print("=" * 50)
        
    except Exception as e:
        print(f"\n❌ Setup failed: {e}")
        conn.rollback()
        sys.exit(1)
    finally:
        cur.close()
        conn.close()

if __name__ == "__main__":
    main()