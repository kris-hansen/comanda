# Use the official PostgreSQL image from the Docker Hub
FROM postgres:latest

# Set environment variables for the PostgreSQL database
ENV POSTGRES_USER=myuser
ENV POSTGRES_PASSWORD=mypassword
ENV POSTGRES_DB=mydatabase

# Copy the SQL file to the Docker image
COPY simpledb.sql /docker-entrypoint-initdb.d/

# Expose the PostgreSQL port
EXPOSE 5432