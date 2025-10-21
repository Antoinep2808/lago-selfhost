FROM getlago/api:v1.33.2

# Expose the Lago API port
EXPOSE 3000

# Default environment
ENV LAGO_ENV=production

# Start Lago API
CMD ["/bin/sh", "-c", "./scripts/start.api.sh"]