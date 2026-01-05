# Sample Technical Specification: User Authentication System

## Overview

Implement a complete user authentication system for the application. The system should support email/password authentication, session management, and basic security features.

## Requirements

### 1. User Registration

Users should be able to register with:
- Email address (unique, validated format)
- Password (minimum 8 characters, must include uppercase, lowercase, number)
- Display name (optional)

The system should:
- Validate email format and uniqueness
- Hash passwords using bcrypt
- Send confirmation email
- Create user record in database

### 2. User Login

Users should be able to log in with email and password:
- Validate credentials against stored hash
- Generate JWT access token (1 hour expiry)
- Generate refresh token (7 day expiry)
- Store session in database
- Return tokens to client

### 3. Session Management

The system should manage active sessions:
- Validate JWT tokens on protected routes
- Refresh expired access tokens using refresh token
- Allow users to view active sessions
- Allow users to revoke specific sessions
- Automatic cleanup of expired sessions

### 4. Password Reset

Users should be able to reset forgotten passwords:
- Request reset via email
- Generate secure reset token (1 hour expiry)
- Validate reset token
- Update password and invalidate all sessions

### 5. Security Features

Implement basic security measures:
- Rate limiting on login attempts (5 per minute)
- Account lockout after 10 failed attempts
- Secure cookie settings (httpOnly, secure, sameSite)
- CSRF protection for forms
- Audit logging for auth events

## Technical Constraints

- Backend: Go with Gin framework
- Database: PostgreSQL
- Token format: JWT with RS256
- Password hashing: bcrypt with cost 12
- All endpoints must be RESTful

## API Endpoints

```
POST /auth/register    - Create new user
POST /auth/login       - Authenticate user
POST /auth/logout      - End session
POST /auth/refresh     - Refresh access token
GET  /auth/sessions    - List active sessions
DELETE /auth/sessions/:id - Revoke session
POST /auth/password/forgot - Request reset
POST /auth/password/reset  - Reset password
```

## Out of Scope

- OAuth/social login (future phase)
- Two-factor authentication (future phase)
- Admin user management (separate spec)
