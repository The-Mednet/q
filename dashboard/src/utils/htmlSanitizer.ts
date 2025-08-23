export function sanitizeHtml(html: string): string {
  // Simple HTML sanitizer - removes script tags and dangerous attributes
  // For production, consider using a proper library like DOMPurify
  
  if (typeof window === 'undefined') {
    // On server side, return empty string for safety
    return '';
  }
  
  // Remove script tags and their content
  let sanitized = html.replace(/<script[^>]*>.*?<\/script>/gi, '');
  
  // Remove dangerous event handlers
  sanitized = sanitized.replace(/on\w+="[^"]*"/gi, '');
  sanitized = sanitized.replace(/on\w+='[^']*'/gi, '');
  
  // Remove javascript: protocol links
  sanitized = sanitized.replace(/href\s*=\s*["']javascript:[^"']*["']/gi, 'href="#"');
  
  // Remove style attributes that might contain expressions
  sanitized = sanitized.replace(/style\s*=\s*["'][^"']*expression[^"']*["']/gi, '');
  
  return sanitized;
}